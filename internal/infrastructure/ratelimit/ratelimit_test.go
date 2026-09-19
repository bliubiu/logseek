package ratelimit

import (
	"context"
	"testing"
	"time"
)

func TestDisabled(t *testing.T) {
	l := New(0)
	if l.Enabled() {
		t.Fatal("0 应关闭限速")
	}
	start := time.Now()
	for i := 0; i < 100; i++ {
		if err := l.Wait(context.Background(), 1<<20); err != nil {
			t.Fatal(err)
		}
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("关闭时不应等待")
	}
}

func TestEnabledThrottles(t *testing.T) {
	// 1MiB/s，写 3 次 512KiB → 至少等约 1s
	l := New(1 << 20)
	start := time.Now()
	for i := 0; i < 3; i++ {
		if err := l.Wait(context.Background(), 512<<10); err != nil {
			t.Fatal(err)
		}
	}
	if time.Since(start) < 400*time.Millisecond {
		t.Fatalf("限速未生效，耗时 %s", time.Since(start))
	}
}
