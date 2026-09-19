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
