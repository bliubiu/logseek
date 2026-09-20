// Package probe 实现大文件头尾随机预检（只读）。
package probe

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/bliubiu/logseek/internal/domain/errkind"
	"github.com/bliubiu/logseek/internal/domain/timefmt"
)

// 默认采样窗口大小。
const sampleWindow = 64 << 10

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
	rep.HeadSamples = headLines(head, 5)
	rep.TailSamples = tailLines(tail, 5)

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

	if t, ok := timefmt.ParseWith(layout.Layout, firstNonEmpty(rep.HeadSamples)); ok {
		rep.FirstTime = t.Format("2006-01-02 15:04:05")
	}
	if t, ok := timefmt.ParseWith(layout.Layout, lastNonEmpty(rep.TailSamples)); ok {
		rep.LastTime = t.Format("2006-01-02 15:04:05")
	}
	return rep, nil
}

func firstNonEmpty(ss []string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func lastNonEmpty(ss []string) string {
	for i := len(ss) - 1; i >= 0; i-- {
		if strings.TrimSpace(ss[i]) != "" {
			return ss[i]
		}
	}
	return ""
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

func headLines(b []byte, n int) []string {
	s := bufio.NewScanner(strings.NewReader(string(b)))
	s.Buffer(make([]byte, 0, 4096), 1<<20)
	var out []string
	for s.Scan() && len(out) < n {
		line := s.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

func tailLines(b []byte, n int) []string {
	lines := headLines(b, n*3)
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
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
