package timefmt

import (
	"fmt"
	"strings"
	"time"
)

// 常见日志时间格式候选（按优先级）。
var candidates = []string{
	"2006-01-02 15:04:05.000",
	"2006-01-02 15:04:05.000000",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05.000Z07:00",
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02T15:04:05",
	"01/02/2006 15:04:05",
	"02/Jan/2006:15:04:05 -0700",
	"Jan 02 15:04:05",
	"2006-01-02",
}

// Layout 表示已识别或用户指定的时间格式。
type Layout struct {
	Layout string
	// FromUser 表示用户显式指定，优先生效。
	FromUser bool
}

// Detect 对样例行做时间格式探测，返回得分最高的布局；失败返回 error。
func Detect(samples []string) (Layout, error) {
	if len(samples) == 0 {
		return Layout{}, fmt.Errorf("没有可用于时间格式探测的样例行")
	}
	best := ""
	bestHit := 0
	for _, layout := range candidates {
		hit := 0
		for _, line := range samples {
			if _, ok := ParseWith(layout, line); ok {
				hit++
			}
		}
		if hit > bestHit {
			bestHit = hit
			best = layout
		}
	}
	if best == "" || bestHit == 0 {
		return Layout{}, fmt.Errorf("未能识别时间格式，请使用 --time-format 显式指定或仅做内容检索")
	}
	return Layout{Layout: best}, nil
}

// ParseWith 用指定布局尝试从行中解析时间（允许行首前缀与行尾后缀）。
// 无时区信息的布局一律按本地时区解释，与项目日志时间戳约定一致。
func ParseWith(layout, line string) (time.Time, bool) {
	line = strings.TrimSpace(line)
	if line == "" || layout == "" {
		return time.Time{}, false
	}

	// 1) 整行精确匹配（优先本地时区）
	if t, err := time.ParseInLocation(layout, line, time.Local); err == nil {
		return t, true
	}
	if t, err := time.Parse(layout, line); err == nil {
		return t, true
	}

	// 2) 取恰好 len(layout) 长度的窗口滑动（时间戳长度与 layout 固定对应）
	need := len(layout)
	if need > 0 && len(line) >= need {
		maxStart := len(line) - need
		if maxStart > 64 {
			maxStart = 64
		}
		for i := 0; i <= maxStart; i++ {
			c := line[i]
			if c < '0' && c != 'J' && c != 'j' {
				continue
			}
			cand := line[i : i+need]
			if t, err := time.ParseInLocation(layout, cand, time.Local); err == nil {
				return t, true
			}
			if t, err := time.Parse(layout, cand); err == nil {
				return t, true
			}
		}
	}

	// 3) 前缀截断到 layout 及其变体长度（毫秒可选场景）
	for _, n := range []int{len(layout), len(layout) + 1, len(layout) - 3, 19, 23, 10} {
		if n <= 0 || n > len(line) {
			continue
		}
		cand := line[:n]
		if t, err := time.ParseInLocation(layout, cand, time.Local); err == nil {
			return t, true
		}
		if t, err := time.Parse(layout, cand); err == nil {
			return t, true
		}
		if idx := indexDigit(line); idx >= 0 && idx+n <= len(line) {
			cand2 := line[idx : idx+n]
			if t, err := time.ParseInLocation(layout, cand2, time.Local); err == nil {
				return t, true
			}
			if t, err := time.Parse(layout, cand2); err == nil {
				return t, true
			}
		}
	}
	return time.Time{}, false
}

func indexDigit(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			return i
		}
	}
	return -1
}

// ParseLine 使用布局解析行，优先本地时区语义（与项目日志时间戳一致）。
func ParseLine(l Layout, line string) (time.Time, error) {
	if t, ok := ParseWith(l.Layout, line); ok {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("无法按格式 %q 解析时间", l.Layout)
}

// Candidates 返回只读候选列表副本（测试/预检展示用）。
func Candidates() []string {
	out := make([]string, len(candidates))
	copy(out, candidates)
	return out
}
