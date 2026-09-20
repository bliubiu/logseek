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
	// Oracle alert 日志：Sat Mar 16 16:11:32 2019
	// 必须优先于无年份的 "Jan 02 15:04:05"，否则会丢失年份退化识别。
	"Mon Jan 02 15:04:05 2006",
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
	// 快速失败：不含数字的行不可能含时间戳（性能护栏）。
	if !hasDigit(line) {
		return time.Time{}, false
	}

	ft := firstTokenOf(layout)

	// 单个窗口精确解析；先用低成本的布局首 token 预筛拦掉无关注定失败的行。
	tryWindow := func(s string) (time.Time, bool) {
		if !satisfiesFirstToken(s, ft) {
			return time.Time{}, false
		}
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, true
		}
		return time.Time{}, false
	}

	// 1) 整行精确匹配（优先本地时区）
	if t, ok := tryWindow(line); ok {
		return t, true
	}

	// 2) 取恰好 len(layout) 长度的窗口滑动（时间戳长度与 layout 固定对应）；
	//    窗口起点要求与布局首 token 字符类一致，避免对无关字符串调用解析。
	need := len(layout)
	if need > 0 && len(line) >= need {
		maxStart := len(line) - need
		if maxStart > 64 {
			maxStart = 64
		}
		for i := 0; i <= maxStart; i++ {
			if !windowStartOK(line[i], ft) {
				continue
			}
			if t, ok := tryWindow(line[i : i+need]); ok {
				return t, true
			}
		}
	}

	// 3) 前缀截断到 layout 及其变体长度（毫秒可选场景）
	for _, n := range []int{len(layout), len(layout) + 1, len(layout) - 3, 19, 23, 10} {
		if n <= 0 || n > len(line) {
			continue
		}
		if t, ok := tryWindow(line[:n]); ok {
			return t, true
		}
		if idx := indexDigit(line); idx >= 0 && idx+n <= len(line) {
			if t, ok := tryWindow(line[idx : idx+n]); ok {
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

func hasDigit(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			return true
		}
	}
	return false
}

// windowStartKind 窗口起点允许的字符类。
type windowStartKind byte

const (
	startDigit  windowStartKind = 'd'
	startLetter windowStartKind = 'a'
)

// firstToken 描述布局首个 token 的预筛信息（字节级，热路径无分配）。
type firstToken struct {
	kind    windowStartKind
	n       int        // 「数字」token 的连续位数；字母 token 为 0
	cls     tokenClass // 「星期/月份」token 的语义类别；其它为 0
	haveLit bool
}

// tokenClass 首 token 语义分类。
type tokenClass byte

const (
	tokenOther   tokenClass = 0
	tokenMonth   tokenClass = 1
	tokenWeekday tokenClass = 2
)

// foldAscii 折叠 ASCII 大写（月份/星期缩写比较用）。
func foldAscii(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 32
	}
	return c
}

// tokenClassAt 返回三字母缩写实例的语义类别（月/周/其它）。
// 单窗口字节比较，无分配；月度/周日缩写均大小写不敏感。
func tokenClassAt(a, b, c byte) tokenClass {
	a = foldAscii(a)
	b = foldAscii(b)
	c = foldAscii(c)
	switch a {
	case 'j':
		if b == 'a' && c == 'n' {
			return tokenMonth
		}
		if b == 'u' {
			if c == 'n' || c == 'l' {
				return tokenMonth
			}
		}
	case 'f':
		if b == 'e' && c == 'b' {
			return tokenMonth
		}
		if b == 'r' && c == 'i' {
			return tokenWeekday
		}
	case 'm':
		if b == 'a' {
			if c == 'r' || c == 'y' {
				return tokenMonth
			}
		}
		if b == 'o' && c == 'n' {
			return tokenWeekday
		}
	case 'a':
		if b == 'p' && c == 'r' {
			return tokenMonth
		}
		if b == 'u' && c == 'g' {
			return tokenMonth
		}
	case 's':
		if b == 'e' && c == 'p' {
			return tokenMonth
		}
		if b == 'u' && c == 'n' {
			return tokenWeekday
		}
		if b == 'a' && c == 't' {
			return tokenWeekday
		}
	case 'o':
		if b == 'c' && c == 't' {
			return tokenMonth
		}
	case 'n':
		if b == 'o' && c == 'v' {
			return tokenMonth
		}
	case 'd':
		if b == 'e' && c == 'c' {
			return tokenMonth
		}
	case 't':
		if b == 'u' && c == 'e' {
			return tokenWeekday
		}
		if b == 'h' && c == 'u' {
			return tokenWeekday
		}
	case 'w':
		if b == 'e' && c == 'd' {
			return tokenWeekday
		}
	}
	return tokenOther
}

// firstTokenOf 提取布局首 token 的预筛信息。
func firstTokenOf(layout string) firstToken {
	t := firstToken{kind: startDigit}
	i := 0
	for i < len(layout) && layout[i] == ' ' {
		i++
	}
	if i >= len(layout) {
		return t
	}
	c0 := layout[i]
	if c0 >= '0' && c0 <= '9' {
		n := 0
		for i < len(layout) && layout[i] >= '0' && layout[i] <= '9' {
			n++
			i++
		}
		t.n = n
		return t
	}
	if (c0 >= 'A' && c0 <= 'Z') || (c0 >= 'a' && c0 <= 'z') {
		t.kind = startLetter
		j := i
		for j < len(layout) {
			c := layout[j]
			if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
				j++
			} else {
				break
			}
		}
		if j-i >= 3 {
			cls := tokenClassAt(layout[i], layout[i+1], layout[i+2])
			if cls != tokenOther {
				t.cls = cls
				t.haveLit = true
			}
		}
		return t
	}
	return t
}

// windowStartOK 判定窗口起点字符是否符合布局首 token 的字符类。
func windowStartOK(c byte, ft firstToken) bool {
	if ft.kind == startDigit {
		return c >= '0' && c <= '9'
	}
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

// satisfiesFirstToken 低成本预筛：候选串首部是否能匹配布局首 token。
// 字节级比较，不产生任何字符串分配。
func satisfiesFirstToken(s string, ft firstToken) bool {
	if s == "" {
		return false
	}
	if ft.kind == startDigit {
		n := ft.n
		if n == 0 {
			n = 1
		}
		if len(s) < n {
			return false
		}
		for i := 0; i < n; i++ {
			if s[i] < '0' || s[i] > '9' {
				return false
			}
		}
		return true
	}
	if ft.haveLit {
		if len(s) < 3 {
			return false
		}
		// 与布局首 token 语义同类（月份对月份、星期对星期）即放行，
		// 对齐 time.Parse 允许任意月/周缩写解析的行为。
		return tokenClassAt(s[0], s[1], s[2]) == ft.cls
	}
	// 其它字母 token：仅要求首字符为字母。
	return windowStartOK(s[0], ft)
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
