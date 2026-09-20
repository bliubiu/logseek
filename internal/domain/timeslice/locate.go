package timeslice

import (
	"bytes"
	"fmt"
	"sort"
	"time"

	"github.com/bliubiu/logseek/internal/domain/timefmt"
)

// ReaderAt 稀疏定位所需随机读接口（与 probe 一致；域内不依赖文件系统实现）。
type ReaderAt interface {
	ReadAt(p []byte, off int64) (int, error)
	Size() (int64, error)
}

// SpanMode 定位模式。
type SpanMode string

const (
	// SpanSeek 定位成功，可按区间 [StartOff, EndOff) 快速扫描。
	SpanSeek SpanMode = "span-seek"
	// FullScan 无法可靠定位（时间戳过少/乱序/文件过小），回退全文件扫描。
	FullScan SpanMode = "full-scan"
)

// Span 定位结果。Mode 非 SpanSeek 时忽略偏移。
type Span struct {
	Mode SpanMode
	// StartOff、EndOff 为半开 [StartOff, EndOff)。
	StartOff int64
	EndOff   int64
	// Indexed 采样到的实时间戳数（诊断）。
	Indexed int
	// Reason 回退或定位结果的原因说明；Mode=FullScan 时为回退理由，
	// 此时 error 为 nil，调用方应先看 Mode 再决定是否整文件扫描。
	Reason string
}

// IsEmpty 空区间（窗口早于/晚于文件时间范围）。
func (s Span) IsEmpty() bool { return s.StartOff == s.EndOff }

// LocateOptions 采样参数。
type LocateOptions struct {
	// Step 采样步长（字节）。
	Step int64
	// Probe 每采样点探测字节。
	Probe int64
	// Refine 边界精化伸读上限（每点 Probe 字节的轮数）。
	// 当前保守块粒度定位不伸读，该字段保留供后续恢复行级精化时使用。
	Refine int
	// Max 最多采样点数（保护）。
	Max int
}

// DefaultLocateOptions 默认参数：8MiB 步长、64KiB 探测、4 轮伸读、4096 采样点。
func DefaultLocateOptions() LocateOptions {
	return LocateOptions{
		Step:   8 << 20,
		Probe:  64 << 10,
		Refine: 4,
		Max:    4096,
	}
}

// entry 采样点：块起点偏移及该点（经沿用后的）时间。
type entry struct {
	off int64
	t   time.Time
}

// Locate 对时间近似单调的日志做「稀疏采样二分 + 边界外扩」定位。
//
// 边界语义为保守块粒度：返回的 [StartOff, EndOff) 是采样块对齐的超集，
// 起止块整体纳入，宁可多扫不漏；越界行由调用方的时间过滤器再筛掉。
// 因此它天然满足"覆盖所有落入 [win.Start, win.End) 的行"，但不保证是最小区间。
//
// 前提（时间近似单调）不成立时回退 FullScan： spans 给出 Mode=FullScan 与 Reason，
// error 保持 nil，由调用方整文件扫描，语义不变。
func Locate(ra ReaderAt, layout timefmt.Layout, win Window, opts LocateOptions) (Span, error) {
	if !win.Start.Before(win.End) {
		return Span{}, fmt.Errorf("时间窗口非法：起始时间必须早于结束时间")
	}
	o := opts
	if o.Step <= 0 {
		o.Step = DefaultLocateOptions().Step
	}
	if o.Probe <= 0 {
		o.Probe = DefaultLocateOptions().Probe
	}
	if o.Refine <= 0 {
		o.Refine = DefaultLocateOptions().Refine
	}
	if o.Max <= 0 {
		o.Max = DefaultLocateOptions().Max
	}

	size, err := ra.Size()
	if err != nil {
		return Span{}, fmt.Errorf("无法获取文件大小：%w", err)
	}
	if size < o.Step*2 {
		return Span{
			Mode:   FullScan,
			Reason: fmt.Sprintf("文件过小（%.1fMiB），无法稀疏定位", float64(size)/(1<<20)),
		}, nil
	}

	// 采样：逐块读头部，寻找时间戳；无时间戳块沿用前一采样时间。
	entries := make([]entry, 0, 32)
	carry := time.Time{}
	indexed := 0
	seenNonZero := 0
	for off := int64(0); off < size && len(entries) < o.Max; off += o.Step {
		head := make([]byte, o.Probe)
		n := readAt(ra, head, off, size)
		if n > 0 {
			if t, ok := firstTimestamp(head[:n], layout); ok {
				carry = t
				indexed++
			}
		}
		node := entry{off: off, t: carry}
		if !node.t.IsZero() {
			seenNonZero++
		}
		entries = append(entries, node)
	}
	// 有效性门槛：至少两个已知时间量且指示可沿用的采样点足够。
	if indexed < 2 || seenNonZero < 2 {
		return Span{
			Mode:   FullScan,
			Reason: fmt.Sprintf("日志时间戳过少（%d），无法稀疏定位", indexed),
		}, nil
	}

	// 乱序回退：任一已知时间回退即不可靠，整文件扫描保持语义。
	for i := 1; i < len(entries); i++ {
		p, c := entries[i-1].t, entries[i].t
		if p.IsZero() || c.IsZero() {
			continue
		}
		if c.Before(p) {
			return Span{Mode: FullScan, Reason: "日志时间戳乱序，回退全文件扫描"}, nil
		}
	}

	firstT := firstReal(entries)
	lastT := lastReal(entries)
	if !win.End.After(firstT) {
		// 窗口整体早于（含等于）文件最早时间：文件头部无时间戳的行无法排除，
		// 保守起见整文件扫描，由过滤器筛掉（命中集为空，扫描量换确定性）。
		return Span{Mode: SpanSeek, StartOff: 0, EndOff: size, Indexed: indexed,
			Reason: "窗口早于文件时间范围，回退全文件区间"}, nil
	}
	if !win.Start.Before(lastT) {
		// 窗口整体晚于（含等于）文件最晚时间：区间落在文件尾，扫描结果为空。
		return Span{Mode: SpanSeek, StartOff: size, EndOff: size, Indexed: indexed,
			Reason: "窗口晚于文件时间范围，结果为空"}, nil
	}

	// 起点块：二分找首个采样时间 ≥ win.Start 的块，取其前一块起点。
	// 保守语义——窗口首行可能藏于上一块内部（含无时间戳的巨区沿用场景）。
	idxS := lowerBound(entries, win.Start)
	if idxS >= len(entries) {
		idxS = len(entries) - 1
	}
	if idxS > 0 {
		idxS--
	}
	startOff := entries[idxS].off

	// 终点块：首个采样时间 ≥ win.End 的块，取整块末尾（该块仍可能含窗口内的行）。
	// idxE 越界说明窗口延伸至文件尾。
	idxE := lowerBound(entries, win.End)
	endOff := size
	if idxE < len(entries) {
		endOff = entries[idxE].off + o.Step
		if endOff > size {
			endOff = size
		}
	}
	if endOff < startOff {
		return Span{Mode: FullScan, Reason: "边界外扩异常，回退全文件扫描"}, nil
	}
	return Span{Mode: SpanSeek, StartOff: startOff, EndOff: endOff, Indexed: indexed}, nil
}

// readAt 随机读，返回实际读到字节数。
func readAt(ra ReaderAt, buf []byte, off, size int64) int {
	limit := int64(len(buf))
	if off+limit > size {
		limit = size - off
	}
	if limit <= 0 {
		return 0
	}
	buf = buf[:limit]
	total := 0
	for total < len(buf) {
		n, err := ra.ReadAt(buf[total:], off+int64(total))
		total += n
		if err != nil {
			break
		}
	}
	return total
}

// firstTimestamp 探测窗口内逐行查找首个可解析时间戳。
func firstTimestamp(head []byte, layout timefmt.Layout) (time.Time, bool) {
	if t, ok := parseHeadDate(head, layout); ok {
		return t, true
	}
	return time.Time{}, false
}

// parseHeadDate 在 head 内逐行找首个（可解析）时间戳。
func parseHeadDate(head []byte, layout timefmt.Layout) (time.Time, bool) {
	pos := 0
	for pos < len(head) {
		idx := bytes.IndexByte(head[pos:], '\n')
		if idx < 0 {
			break
		}
		line := head[pos : pos+idx]
		pos += idx + 1
		if t, ok := timefmt.ParseWith(layout.Layout, string(line)); ok {
			return t, true
		}
	}
	return time.Time{}, false
}

// firstReal 首个非零时间。
func firstReal(es []entry) time.Time {
	for _, e := range es {
		if !e.t.IsZero() {
			return e.t
		}
	}
	return time.Time{}
}

// lastReal 最后一个非零时间。
func lastReal(es []entry) time.Time {
	for i := len(es) - 1; i >= 0; i-- {
		if !es[i].t.IsZero() {
			return es[i].t
		}
	}
	return time.Time{}
}

// lowerBound 首个 node 时间 ≥ t 的下标；全部更早返回 len。
func lowerBound(es []entry, t time.Time) int {
	return sort.Search(len(es), func(i int) bool {
		return !es[i].t.Before(t)
	})
}
