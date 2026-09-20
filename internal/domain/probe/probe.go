// Package probe 实现大文件头尾随机预检（只读）。
package probe

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bliubiu/logseek/internal/domain/errkind"
	"github.com/bliubiu/logseek/internal/domain/timefmt"
)

// 默认采样窗口大小。
const (
	sampleWindow = 64 << 10
	// sampleLines 头尾样例展示行数。
	sampleLines = 5
	// tailProbeMax 末尾时间戳回溯上限（额外向前探的窗口数）。
	// Oracle alert 等日志末尾常为多行续写（无时间戳），需在窗口内逆向查找，
	// 窗口内仍找不到时再向前扩展，避免「可切片」却查不到末条时间。
	tailProbeMax = 8
)

// ReaderAt 提交与 os.File 兼容的随机读接口。
type ReaderAt interface {
	ReadAt(p []byte, off int64) (n int, err error)
	Size() (int64, error)
}

// Report 预检报告。
type Report struct {
	SizeBytes      int64    `json:"size_bytes"`
	Encoding       string   `json:"encoding"`
	TimeLayout     string   `json:"time_layout"`
	TimeDetected   bool     `json:"time_detected"`
	FirstTime      string   `json:"first_time,omitempty"`
	LastTime       string   `json:"last_time,omitempty"`
	HeadSamples    []string `json:"head_samples"`
	TailSamples    []string `json:"tail_samples"`
	LineBreakStyle string   `json:"line_break"`
	Sliceable      bool     `json:"sliceable"`
	Hint           string   `json:"hint,omitempty"`
}

// Detect 执行头尾采样探测。
func Detect(r ReaderAt) (Report, error) {
	size, err := r.Size()
	if err != nil {
		return Report{}, errkind.Wrap(errkind.KindNotFound, fmt.Errorf("无法获取文件大小：%w", err))
	}
	rep := Report{SizeBytes: size}
	if size == 0 {
		return Report{}, errkind.Wrap(errkind.KindProbeFailed, fmt.Errorf("文件为空，无法预检"))
	}

	head, err := readWindow(r, 0, sampleWindow, size)
	if err != nil {
		return Report{}, errkind.Wrap(errkind.KindNotFound, fmt.Errorf("读取文件头部失败：%w", err))
	}
	tailOff := int64(0)
	if size > sampleWindow {
		tailOff = size - sampleWindow
	}
	tail, err := readWindow(r, tailOff, sampleWindow, size)
	if err != nil {
		return Report{}, errkind.Wrap(errkind.KindNotFound, fmt.Errorf("读取文件尾部失败：%w", err))
	}

	rep.Encoding = detectEncoding(append([]byte{}, head...))
	rep.LineBreakStyle = detectBreak(head)
	rep.HeadSamples = takeLines(allLines(head), sampleLines, false)
	// 尾部窗口起点通常落在一行中间，需先丢弃首行残片，再取窗口末尾若干行，
	// 否则「尾部样例」展示的其实只是窗口开头的内容。
	rep.TailSamples = takeLines(allLines(dropPartialLine(tail, tailOff > 0)), sampleLines, true)

	samples := append(append([]string{}, rep.HeadSamples...), rep.TailSamples...)
	layout, derr := timefmt.Detect(samples)
	if derr != nil {
		rep.Sliceable = false
		rep.Hint = derr.Error()
		return rep, nil
	}
	rep.TimeDetected = true
	rep.TimeLayout = layout.Layout
	rep.Sliceable = true

	// 首尾时间在完整窗口内按行查找，而非只看样例行：
	// 多行日志（如 Oracle alert）的时间戳独占一行，样例行本身常常无时间。
	rep.FirstTime = fmtTime(firstParsable(head, layout.Layout))
	rep.LastTime = fmtTime(lastParsable(r, size, tailOff, layout.Layout))
	return rep, nil
}

// firstParsable 在缓冲区内自前向后找首个可解析时间戳。
func firstParsable(b []byte, layout string) (time.Time, bool) {
	for _, l := range allLines(b) {
		if t, ok := timefmt.ParseWith(layout, l); ok {
			return t, true
		}
	}
	return time.Time{}, false
}

// lastParsable 自文件尾向前找最后一个可解析时间戳。
//
// 先在尾部窗口内逆向扫描；若窗口全是续行（无时间戳），再向前扩展若干窗口，
// 直到命中或达到 tailProbeMax 上限。
func lastParsable(r ReaderAt, size, tailOff int64, layout string) (time.Time, bool) {
	off := tailOff
	for i := 0; i <= tailProbeMax; i++ {
		b, err := readWindow(r, off, sampleWindow, size)
		if err != nil || len(b) == 0 {
			return time.Time{}, false
		}
		lines := allLines(dropPartialLine(b, i == 0 && off > 0))
		for j := len(lines) - 1; j >= 0; j-- {
			if t, ok := timefmt.ParseWith(layout, lines[j]); ok {
				return t, true
			}
		}
		if off == 0 {
			return time.Time{}, false
		}
		off -= sampleWindow
		if off < 0 {
			off = 0
		}
	}
	return time.Time{}, false
}

// fmtTime 格式化时间，零值返回空串。
func fmtTime(t time.Time, ok bool) string {
	if !ok {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}

// allLines 切出全部非空可见行（跳过纯空白行）。
func allLines(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	s := bufio.NewScanner(strings.NewReader(string(b)))
	s.Buffer(make([]byte, 0, 4096), 1<<20)
	var out []string
	for s.Scan() {
		l := strings.TrimRight(s.Text(), "\r")
		if strings.TrimSpace(l) == "" {
			continue
		}
		out = append(out, l)
	}
	return out
}

// takeLines 从头或尾取 n 行；backward 为真时取末尾 n 行。
func takeLines(lines []string, n int, backward bool) []string {
	if len(lines) <= n {
		return lines
	}
	if backward {
		return lines[len(lines)-n:]
	}
	return lines[:n]
}

// dropPartialLine 丢弃起始点半行残片；随机窗口起点常落在某行中间。
// 仅当窗口确实不是从文件开头读起时才丢弃，否则会丢掉真实的最后一行。
func dropPartialLine(b []byte, partial bool) []byte {
	if !partial {
		return b
	}
	if idx := bytes.IndexByte(b, '\n'); idx >= 0 && idx+1 < len(b) {
		return b[idx+1:]
	}
	return b
}

func readWindow(r ReaderAt, off, n, size int64) ([]byte, error) {
	if off >= size {
		return nil, nil
	}
	if off+n > size {
		n = size - off
	}
	buf := make([]byte, n)
	read := int64(0)
	for read < n {
		m, err := r.ReadAt(buf[read:], off+read)
		read += int64(m)
		if err != nil {
			if err == io.EOF && read > 0 {
				break
			}
			if read > 0 {
				break
			}
			return nil, err
		}
	}
	return buf[:read], nil
}

func detectEncoding(b []byte) string {
	if len(b) == 0 {
		return "unknown"
	}
	if utf8.Valid(b) {
		// 若含 BOM 则标注。
		if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
			return "utf-8-bom"
		}
		return "utf-8"
	}
	// 粗粒度 GBK/GB18030 启发：非法 UTF-8 且高位字节常见。
	high := 0
	for _, c := range b {
		if c >= 0x80 {
			high++
		}
	}
	if high > 0 {
		return "gbk"
	}
	return "unknown"
}

func detectBreak(b []byte) string {
	if len(b) == 0 {
		return "unknown"
	}
	if strings.Contains(string(b), "\r\n") {
		return "crlf"
	}
	if strings.Contains(string(b), "\n") {
		return "lf"
	}
	if strings.Contains(string(b), "\r") {
		return "cr"
	}
	return "none"
}

// Format 中文可读报告。
func (r Report) Format() string {
	var sb strings.Builder
	sb.WriteString("=== logSeek 预检报告 ===\n")
	fmt.Fprintf(&sb, "文件大小: %d 字节\n", r.SizeBytes)
	fmt.Fprintf(&sb, "编码: %s\n", r.Encoding)
	fmt.Fprintf(&sb, "换行风格: %s\n", r.LineBreakStyle)
	if r.TimeDetected {
		fmt.Fprintf(&sb, "时间格式: %s\n", r.TimeLayout)
		fmt.Fprintf(&sb, "首条时间: %s\n", r.FirstTime)
		fmt.Fprintf(&sb, "末条时间: %s\n", r.LastTime)
		sb.WriteString("可时间切片: 是\n")
	} else {
		sb.WriteString("可时间切片: 否\n")
		fmt.Fprintf(&sb, "提示: %s\n", r.Hint)
	}
	sb.WriteString("头部样例:\n")
	for _, l := range r.HeadSamples {
		sb.WriteString("  " + truncate(l, 160) + "\n")
	}
	sb.WriteString("尾部样例:\n")
	for _, l := range r.TailSamples {
		sb.WriteString("  " + truncate(l, 160) + "\n")
	}
	return sb.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
