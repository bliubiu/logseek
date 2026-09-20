// Package mask 按 00-安全设计 实现输出脱敏。
package mask

import (
	"regexp"
	"strings"
)

var (
	// 形似 IPv4 的数值串，含可选的后续数值段（版本号形态，如 11.2.0.4.0）。
	// 不使用 \b 之外的定界捕获：相邻 IP 之间可能只隔一个空格，
	// 若把空格算进匹配会导致后一个被跳过。
	reIPLike = regexp.MustCompile(`\b((?:\d{1,3}\.){3}\d{1,3}(?:\.\d+)*)\b`)
	// 手机号（大陆）
	rePhone = regexp.MustCompile(`\b1[3-9]\d{9}\b`)
	// 身份证 18 位
	reIDCard = regexp.MustCompile(`\b\d{17}[\dXx]\b`)
	// 邮箱
	reEmail = regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`)
)

// Apply 对文本执行脱敏；不修改原始存储，仅展示层与导出层。
func Apply(s string) string {
	s = maskIPv4In(s)
	s = rePhone.ReplaceAllString(s, "***MASKED_PHONE***")
	s = reIDCard.ReplaceAllString(s, "***MASKED_ID***")
	s = reEmail.ReplaceAllStringFunc(s, func(m string) string {
		at := strings.Index(m, "@")
		if at <= 0 {
			return m
		}
		return m[:1] + "***" + m[at:]
	})
	return s
}

// versionWords 数值串前置的版本语境关键词。
var versionWords = []string{"v", "ver", "version", "release", "build", "版本", "构建"}

// maskIPv4In 对整段文本中的 IPv4 做保留后两段的打码。
//
// 为避免把版本号误判成 IP（Oracle alert 常见 Release 11.2.0.4.0），做三层判定：
//  1. 数值段超过四段（11.2.0.4.0）或前面紧邻版本语境词（v11.2.0.4 / Version 11.2.0.4）→ 视为版本号，放行；
//  2. 每段必须落在 0~255 且无 01/007 这类多余前导零，否则不是合法 IPv4，放行；
//  3. 127.0.0.1 本机回环保留，便于排障。
//
// 已知限制：脱离上下文的纯四段数值（如孤立出现的 11.2.0.4）仍会按 IP 打码，
// 彻底区分需要列名驱动或行首结构信息，属后续改造项。
func maskIPv4In(s string) string {
	loc := reIPLike.FindAllStringSubmatchIndex(s, -1)
	if loc == nil {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 32)
	last := 0
	for _, m := range loc {
		start, end := m[2], m[3]
		tok := s[start:end]
		if strings.Count(tok, ".") >= 4 || precededByVersionWord(s[:start]) {
			continue
		}
		b.WriteString(s[last:start])
		b.WriteString(maskIPv4Token(tok))
		last = end
	}
	b.WriteString(s[last:])
	return b.String()
}

// maskIPv4Token 判定单个数值串是否为 IPv4 并打码。
func maskIPv4Token(tok string) string {
	if tok == "127.0.0.1" {
		return tok
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 4 {
		return tok
	}
	for _, p := range parts {
		if !validOctet(p) {
			return tok
		}
	}
	return "*.*." + parts[2] + "." + parts[3]
}

// validOctet 校验单个 IPv4 段：0~255 且无前导零。
func validOctet(s string) bool {
	if s == "" || len(s) > 3 {
		return false
	}
	if len(s) > 1 && s[0] == '0' {
		return false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return false
		}
		n = n*10 + int(c-'0')
	}
	return n <= 255
}

// precededByVersionWord 判断数值串之前是否紧邻版本语境词。
func precededByVersionWord(prefix string) bool {
	p := strings.ToLower(strings.TrimRight(prefix, " \t"))
	if p == "" {
		return false
	}
	for _, w := range versionWords {
		if strings.HasSuffix(p, w) {
			return true
		}
	}
	return false
}
