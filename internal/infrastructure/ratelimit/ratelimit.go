// Package ratelimit 提供简单顺序读限速（字节/秒）。
package ratelimit

import (
	"context"
	"time"
)

// Limiter 按字节速率限速；bytesPerSec<=0 表示不限速。
type Limiter struct {
	bytesPerSec int64
	last        time.Time
	budget      int64
}

// New 创建限速器。
func New(bytesPerSec int64) *Limiter {
	return &Limiter{bytesPerSec: bytesPerSec}
}

// Wait 消耗 n 字节配额，必要时 sleep。
func (l *Limiter) Wait(ctx context.Context, n int) error {
	if l == nil || l.bytesPerSec <= 0 || n <= 0 {
		return nil
	}
	now := time.Now()
	if l.last.IsZero() {
		l.last = now
		l.budget = 0
	}
	// 按时间补充预算
	elapsed := now.Sub(l.last).Microseconds()
	if elapsed > 0 {
		l.last = now
		// bytes per microsecond
		add := l.bytesPerSec * elapsed / 1_000_000
		l.budget += add
		if l.budget > l.bytesPerSec {
			l.budget = l.bytesPerSec // 突发上限 1 秒量
		}
	}
	l.budget -= int64(n)
	if l.budget >= 0 {
		return nil
	}
	// 需要等待
	need := -l.budget
	sleep := time.Duration(float64(need) / float64(l.bytesPerSec) * float64(time.Second))
	if sleep < time.Millisecond {
		sleep = time.Millisecond
	}
	t := time.NewTimer(sleep)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		l.budget = 0
		return nil
	}
}

// Enabled 是否启用限速。
func (l *Limiter) Enabled() bool {
	return l != nil && l.bytesPerSec > 0
}
