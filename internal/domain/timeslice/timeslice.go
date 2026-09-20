// Package timeslice 实现时间切片过滤。
package timeslice

import (
	"fmt"
	"time"

	"github.com/bliubiu/logseek/internal/domain/timefmt"
)

// Window 时间窗口；约定闭开 [Start, End)。
type Window struct {
	Start time.Time
	End   time.Time
}

// Policy 坏行策略。
type Policy struct {
	// DropBadLines=true：无法确定时间归属的行丢弃并计数（默认）。
	DropBadLines bool
	// StickyTime=true（默认）：无时间戳的续行沿用上一个已知时间戳参与窗口判定。
	//
	// Oracle alert 等多行日志的时间戳独占一行，其后多行报错正文不带时间戳；
	// 若逐行独立判定，这些正文行会因解析失败被整体丢弃（实测丢失 96% 内容）。
	// 开启后正文沿用其所属时间戳，行为与「按日志条目切片」一致。
	// 对每行都带时间戳的普通日志无续行可继承，行为完全不变。
	StickyTime bool
}

// DefaultPolicy 默认策略：丢弃无法确定归属的行，并启用时间戳继承。
func DefaultPolicy() Policy { return Policy{DropBadLines: true, StickyTime: true} }

// Filter 按窗口过滤行；返回是否命中、是否坏行。
type Filter struct {
	Layout timefmt.Layout
	Window Window
	Policy Policy
	// BadLines 无法确定时间归属的行计数。
	// 时间戳继承生效后，仅统计位于文件首条时间戳之前的行（沿用无可继承值）。
	BadLines int64
	// Scanned 扫描行数
	Scanned int64
	// Inherited 通过时间戳继承参与判定的续行计数（诊断用）。
	Inherited int64
	// last 最近一次成功解析的时间；供续行沿用。
	last time.Time
}

// New 创建切片过滤器。
func New(layout timefmt.Layout, start, end time.Time, p Policy) (*Filter, error) {
	if !start.Before(end) {
		return nil, fmt.Errorf("时间窗口非法：起始时间必须早于结束时间")
	}
	return &Filter{
		Layout: layout,
		Window: Window{Start: start, End: end},
		Policy: p,
	}, nil
}

// RelativeWindow 解析相对时间窗口，如 last 30m / last 7d；end=now。
func RelativeWindow(spec string, now time.Time) (start, end time.Time, err error) {
	if spec == "" {
		return time.Time{}, time.Time{}, fmt.Errorf("相对时间不能为空")
	}
	// 支持 last30m last7d last1h 等
	if len(spec) < 2 {
		return time.Time{}, time.Time{}, fmt.Errorf("相对时间格式非法：%s", spec)
	}
	unit := spec[len(spec)-1]
	numStr := spec[:len(spec)-1]
	var n int
	if _, scanErr := fmt.Sscanf(numStr, "%d", &n); scanErr != nil || n <= 0 {
		return time.Time{}, time.Time{}, fmt.Errorf("相对时间格式非法：%s", spec)
	}
	var d time.Duration
	switch unit {
	case 's', 'S':
		d = time.Duration(n) * time.Second
	case 'm', 'M':
		d = time.Duration(n) * time.Minute
	case 'h', 'H':
		d = time.Duration(n) * time.Hour
	case 'd', 'D':
		d = time.Duration(n) * 24 * time.Hour
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("相对时间单位非法：%c（支持 s/m/h/d）", unit)
	}
	end = now
	start = now.Add(-d)
	return start, end, nil
}

// Accept 判断一行是否落入窗口。
//
// 判定顺序：先尝试按行自身时间戳解析；解析失败且启用时间戳继承时，
// 沿用上一个已知时间戳（多行日志的正文行）；两者皆无才计为无法归属的行。
func (f *Filter) Accept(line []byte) bool {
	f.Scanned++
	t, ok := timefmt.ParseWith(f.Layout.Layout, string(line))
	if !ok {
		if f.Policy.StickyTime && !f.last.IsZero() {
			// 续行：沿用所属条目的时间戳。
			f.Inherited++
			t = f.last
		} else {
			f.BadLines++
			return !f.Policy.DropBadLines
		}
	} else {
		f.last = t
	}
	// 闭开 [start, end)
	return !t.Before(f.Window.Start) && t.Before(f.Window.End)
}

// Stats 返回统计。
func (f *Filter) Stats() (scanned, bad int64) {
	return f.Scanned, f.BadLines
}

// InheritedLines 返回通过时间戳继承参与判定的行数。
func (f *Filter) InheritedLines() int64 { return f.Inherited }
