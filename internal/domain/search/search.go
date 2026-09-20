// Package search 实现内容检索：字节扫描优先，FSM 多词，RE2 兜底。
package search

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Combine 多关键词组合方式。
type Combine int

const (
	CombineAnd Combine = iota
	CombineOr
)

// Condition 检索条件。
type Condition struct {
	Keywords      []string
	Combine       Combine
	CaseSensitive bool
	WholeWord     bool
	// Pattern 复杂模式；仅当关键词无法表达时使用，走 RE2。
	Pattern string
	// UseRE2 强制使用 RE2（测试/覆盖）。
	UseRE2 bool
}

// Matcher 抽象匹配器。
type Matcher interface {
	Match(line []byte) bool
	Name() string
}

// Build 构建匹配器：关键词默认字节扫描/AC；复杂模式走 RE2。
func Build(c Condition) (Matcher, error) {
	if c.Pattern != "" && len(c.Keywords) == 0 {
		return newRE2Matcher(c.Pattern, c.CaseSensitive)
	}
	if len(c.Keywords) == 0 && c.Pattern == "" {
		return matchAll{}, nil
	}
	if c.UseRE2 && c.Pattern != "" {
		return newRE2Matcher(c.Pattern, c.CaseSensitive)
	}
	if len(c.Keywords) == 1 && !c.WholeWord {
		return &byteScanMatcher{
			needle:        normalize([]byte(c.Keywords[0]), c.CaseSensitive),
			caseSensitive: c.CaseSensitive,
			or:            c.Combine == CombineOr,
		}, nil
	}
	// 多词：AC 自动机（FSM）。
	words := append([]string{}, c.Keywords...)
	if c.Pattern != "" {
		// 有 pattern 又有 keyword：keyword 用 FSM，pattern 单独 RE2 后与组合。
		// 简化：优先 keywords FSM，忽略同时存在 pattern 的混合（文档约定关键词为主）。
	}
	ac, err := buildAC(words, c.CaseSensitive)
	if err != nil {
		return nil, err
	}
	ac.wholeWord = c.WholeWord
	ac.or = c.Combine == CombineOr
	ac.caseSensitive = c.CaseSensitive
	return ac, nil
}

type matchAll struct{}

func (matchAll) Match([]byte) bool { return true }
func (matchAll) Name() string      { return "全匹配" }

// byteScanMatcher 单关键词滑动窗口字节比较。
type byteScanMatcher struct {
	needle        []byte
	caseSensitive bool
	or            bool
}

func (m *byteScanMatcher) Name() string { return "字节扫描" }

func (m *byteScanMatcher) Match(line []byte) bool {
	if len(m.needle) == 0 {
		return true
	}
	if !m.caseSensitive {
		line = toLowerBytes(line)
	}
	return containsBytes(line, m.needle)
}

func containsBytes(hay, needle []byte) bool {
	if len(needle) == 0 {
		return true
	}
	if len(needle) > len(hay) {
		return false
	}
	first := needle[0]
	n := len(needle)
	for i := 0; i+n <= len(hay); i++ {
		if hay[i] != first {
			continue
		}
		if bytesEqual(hay[i:i+n], needle) {
			return true
		}
	}
	return false
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func normalize(b []byte, caseSensitive bool) []byte {
	if caseSensitive {
		return b
	}
	return toLowerBytes(b)
}

func toLowerBytes(b []byte) []byte {
	out := make([]byte, len(b))
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			out[i] = c + 32
		} else {
			out[i] = c
		}
	}
	// 非 ASCII 不处理大小写（中文无大小写）。
	return out
}

// acMatcher Aho-Corasick 自动机（FSM 多模式）。
type acMatcher struct {
	next          [][256]int
	patterns      [][]byte // 已 normalize
	out           [][]int
	caseSensitive bool
	wholeWord     bool
	or            bool
	fail          []int
}

func buildAC(words []string, caseSensitive bool) (*acMatcher, error) {
	m := &acMatcher{caseSensitive: caseSensitive}
	if len(words) == 0 {
		return nil, fmt.Errorf("关键词列表为空")
	}
	// 构建 trie
	m.next = make([][256]int, 1)
	for i := range m.next[0] {
		m.next[0][i] = -1
	}
	m.out = make([][]int, 1)
	for wi, w := range words {
		if w == "" {
			continue
		}
		nb := normalize([]byte(w), caseSensitive)
		m.patterns = append(m.patterns, nb)
		state := 0
		for _, c := range nb {
			if m.next[state][c] < 0 {
				m.next = append(m.next, [256]int{})
				for i := range m.next[len(m.next)-1] {
					m.next[len(m.next)-1][i] = -1
				}
				m.out = append(m.out, nil)
				m.next[state][c] = len(m.next) - 1
			}
			state = m.next[state][c]
		}
		m.out[state] = append(m.out[state], wi)
	}
	m.fail = make([]int, len(m.next))
	// BFS 构建 fail 指针
	queue := make([]int, 0, len(m.next))
	for c := 0; c < 256; c++ {
		if m.next[0][c] >= 0 {
			child := m.next[0][c]
			m.fail[child] = 0
			queue = append(queue, child)
		} else {
			// 根缺失转移指向 0（简化：匹配时逐步）
		}
	}
	for len(queue) > 0 {
		v := queue[0]
		queue = queue[1:]
		for c := 0; c < 256; c++ {
			u := m.next[v][c]
			if u < 0 {
				continue
			}
			queue = append(queue, u)
			f := m.fail[v]
			for f != 0 && m.next[f][c] < 0 {
				f = m.fail[f]
			}
			if m.next[f][c] >= 0 && m.next[f][c] != u {
				m.fail[u] = m.next[f][c]
			} else {
				m.fail[u] = 0
			}
			m.out[u] = append(m.out[u], m.out[m.fail[u]]...)
		}
	}
	return m, nil
}

func (m *acMatcher) Name() string { return "FSM/AC自动机" }

func (m *acMatcher) Match(line []byte) bool {
	if !m.caseSensitive {
		// 按字节 lower；多字节 UTF-8 中文不受影响。
		// 对 ASCII 字母转换。
	}
	hits := 0
	need := 1
	if !m.or {
		need = len(m.patterns)
	}
	// 记录已命中 pattern，避免 or=false 时重复计数。
	matched := make([]bool, len(m.patterns))
	state := 0
	for i := 0; i < len(line); i++ {
		c := line[i]
		if !m.caseSensitive && c >= 'A' && c <= 'Z' {
			c = c + 32
		}
		for state != 0 && m.next[state][c] < 0 {
			state = m.fail[state]
		}
		if m.next[state][c] >= 0 {
			state = m.next[state][c]
		} else {
			state = 0
		}
		for _, pi := range m.out[state] {
			if pi >= 0 && pi < len(matched) && !matched[pi] {
				matched[pi] = true
				hits++
			}
		}
		if m.or && hits > 0 {
			return true
		}
	}
	if m.or {
		return hits > 0
	}
	// AND：全部 pattern 命中
	for _, ok := range matched {
		if !ok {
			return false
		}
	}
	return hits >= need && need > 0 || (need == 0 && true)
}

// re2Matcher 使用标准库 regexp（RE2 语义，无回溯）。
type re2Matcher struct {
	re interface {
		Match(b []byte) bool
	}
	pattern string
}

func newRE2Matcher(pattern string, caseSensitive bool) (Matcher, error) {
	p := pattern
	if !caseSensitive && !strings.Contains(pattern, "(?i)") {
		p = "(?i)" + pattern
	}
	re, err := compileRE2(p)
	if err != nil {
		return nil, fmt.Errorf("正则模式无效：%w", err)
	}
	return &re2Matcher{re: re, pattern: pattern}, nil
}

func (m *re2Matcher) Name() string { return "RE2兜底" }

func (m *re2Matcher) Match(line []byte) bool {
	return m.re.Match(line)
}

// IsASCII 判断是否纯 ASCII（辅助）。
func IsASCII(s string) bool {
	return utf8.ValidString(s) && strings.IndexFunc(s, func(r rune) bool { return r > 127 }) < 0
}
