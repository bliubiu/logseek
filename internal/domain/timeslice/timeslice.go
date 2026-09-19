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
	// DropBadLines=true：无法解析时间的行丢弃并计数（默认）。
	DropBadLines bool
}

// DefaultPolicy 默认策略。
func DefaultPolicy() Policy { return Policy{DropBadLines: true} }

// Filter 按窗口过滤行；返回是否命中、是否坏行。
type Filter struct {
	Layout timefmt.Layout
	Window Window
	Policy Policy
	// BadLines 坏行计数
	BadLines int64
	// Scanned 扫描行数
	Scanned int64
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

// Accept 判断一行是否落入窗口。
func (f *Filter) Accept(line []byte) bool {
	f.Scanned++
	t, err := timefmt.ParseLine(f.Layout, string(line))
	if err != nil {
		f.BadLines++
		return !f.Policy.DropBadLines
	}
	// 闭开 [start, end)
	return !t.Before(f.Window.Start) && t.Before(f.Window.End)
}

// Stats 返回统计。
func (f *Filter) Stats() (scanned, bad int64) {
	return f.Scanned, f.BadLines
}
