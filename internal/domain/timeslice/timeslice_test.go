package timeslice_test

import (
	"testing"
	"time"

	"github.com/bliubiu/logseek/internal/domain/timefmt"
	"github.com/bliubiu/logseek/internal/domain/timeslice"
)

func mustFilter(t *testing.T, start, end string) *timeslice.Filter {
	t.Helper()
	lay := timefmt.Layout{Layout: "2006-01-02 15:04:05"}
	st, _ := time.ParseInLocation("2006-01-02 15:04:05", start, time.Local)
	en, _ := time.ParseInLocation("2006-01-02 15:04:05", end, time.Local)
	f, err := timeslice.New(lay, st, en, timeslice.DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestWindowHalfOpen(t *testing.T) {
	f := mustFilter(t, "2026-01-01 10:00:00", "2026-01-01 11:00:00")
	// 边界 [start, end)
	if !f.Accept([]byte("2026-01-01 10:00:00 hit start")) {
		t.Error("起始闭区间应命中")
	}
	if f.Accept([]byte("2026-01-01 11:00:00 hit end")) {
		t.Error("结束开区间不应命中")
	}
	if !f.Accept([]byte("2026-01-01 10:30:00 mid")) {
		t.Error("区间内应命中")
	}
	if f.Accept([]byte("2026-01-01 09:59:59 before")) {
		t.Error("区间前不应命中")
	}
}

func TestBadLineDropped(t *testing.T) {
	f := mustFilter(t, "2026-01-01 10:00:00", "2026-01-01 11:00:00")
	if f.Accept([]byte("not a timestamp line")) {
		t.Error("默认坏行应丢弃")
	}
	_, bad := f.Stats()
	if bad != 1 {
		t.Errorf("坏行计数 = %d, 期望 1", bad)
	}
}

func TestInvalidWindow(t *testing.T) {
	lay := timefmt.Layout{Layout: "2006-01-02"}
	now := time.Now()
	if _, err := timeslice.New(lay, now, now.Add(-time.Hour), timeslice.DefaultPolicy()); err == nil {
		t.Fatal("非法窗口应报错")
	}
}

func TestRelativeWindow(t *testing.T) {
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.Local)
	s, e, err := timeslice.RelativeWindow("30m", now)
	if err != nil {
		t.Fatal(err)
	}
	if !e.Equal(now) || s != now.Add(-30*time.Minute) {
		t.Fatalf("%v %v", s, e)
	}
	s2, _, err := timeslice.RelativeWindow("7d", now)
	if err != nil {
		t.Fatal(err)
	}
	if s2 != now.Add(-7*24*time.Hour) {
		t.Fatal("7d 解析错误")
	}
	if _, _, err := timeslice.RelativeWindow("xx", now); err == nil {
		t.Fatal("非法相对时间应报错")
	}
}

func mustFilterWithPolicy(t *testing.T, start, end string, p timeslice.Policy) *timeslice.Filter {
	t.Helper()
	lay := timefmt.Layout{Layout: "2006-01-02 15:04:05"}
	st, _ := time.ParseInLocation("2006-01-02 15:04:05", start, time.Local)
	en, _ := time.ParseInLocation("2006-01-02 15:04:05", end, time.Local)
	f, err := timeslice.New(lay, st, en, p)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// TestStickyTimeInheritsPreviousTimestamp 多行日志：时间戳独占一行，
// 其后正文行无时间戳，应沿用所属时间戳一起命中。
func TestStickyTimeInheritsPreviousTimestamp(t *testing.T) {
	f := mustFilter(t, "2026-01-01 10:00:00", "2026-01-01 11:00:00")
	if !f.Accept([]byte("2026-01-01 10:30:00")) {
		t.Fatal("时间戳行应命中")
	}
	// 后续正文行沿用 10:30，应一并命中
	for _, body := range []string{
		"Errors in file /u01/app/oracle/diag/trace/alert.log",
		"ORA-00600: internal error code, arguments: [1234]",
		"",
	} {
		if !f.Accept([]byte(body)) {
			t.Fatalf("续行应继承时间戳命中: %q", body)
		}
	}
	if got := f.InheritedLines(); got != 3 {
		t.Fatalf("继承行数 = %d, 期望 3", got)
	}
	if _, bad := f.Stats(); bad != 0 {
		t.Fatalf("继承后不应再计坏行, bad = %d", bad)
	}
}

// TestStickyTimeDisabled 关闭继承后退化为逐行独立判定，正文行丢弃。
func TestStickyTimeDisabled(t *testing.T) {
	p := timeslice.DefaultPolicy()
	p.StickyTime = false
	f := mustFilterWithPolicy(t, "2026-01-01 10:00:00", "2026-01-01 11:00:00", p)
	if !f.Accept([]byte("2026-01-01 10:30:00")) {
		t.Fatal("时间戳行应命中")
	}
	if f.Accept([]byte("ORA-00600: internal error code")) {
		t.Fatal("关闭继承后正文行不应命中")
	}
	if got := f.InheritedLines(); got != 0 {
		t.Fatalf("继承行数 = %d, 期望 0", got)
	}
	if _, bad := f.Stats(); bad != 1 {
		t.Fatalf("坏行计数 = %d, 期望 1", bad)
	}
}

// TestStickyTimeLeadingLinesWithoutTimestamp 首条时间戳之前的行无可继承值，计为坏行。
func TestStickyTimeLeadingLinesWithoutTimestamp(t *testing.T) {
	f := mustFilter(t, "2026-01-01 10:00:00", "2026-01-01 11:00:00")
	if f.Accept([]byte("some preamble without timestamp")) {
		t.Fatal("首条时间戳之前的续行不应命中")
	}
	if got := f.InheritedLines(); got != 0 {
		t.Fatalf("继承行数 = %d, 期望 0", got)
	}
	if _, bad := f.Stats(); bad != 1 {
		t.Fatalf("坏行计数 = %d, 期望 1", bad)
	}
}

// TestStickyTimeFollowsWindowBoundary 沿用值跨窗口变动时须同步切换。
func TestStickyTimeFollowsWindowBoundary(t *testing.T) {
	f := mustFilter(t, "2026-01-01 10:00:00", "2026-01-01 11:00:00")

	// 窗口之前：条目与其正文均不命中
	if f.Accept([]byte("2026-01-01 09:00:00")) {
		t.Fatal("窗口前不应命中")
	}
	if f.Accept([]byte("body of 09:00 entry")) {
		t.Fatal("窗口前条目的正文不应命中")
	}
	// 进入窗口：整条目命中
	if !f.Accept([]byte("2026-01-01 10:00:00")) {
		t.Fatal("窗口起点应命中")
	}
	if !f.Accept([]byte("body of 10:00 entry")) {
		t.Fatal("窗口内条目的正文应命中")
	}
	// 越出窗口：整条目不再命中
	if f.Accept([]byte("2026-01-01 11:00:00")) {
		t.Fatal("窗口后不应命中")
	}
	if f.Accept([]byte("body of 11:00 entry")) {
		t.Fatal("窗口后条目的正文不应命中")
	}
}
