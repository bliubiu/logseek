// Package mask 按 00-安全设计 实现输出脱敏。
package mask

import (
	"regexp"
	"strings"
)

var (
	// IPv4：保留后两段
	reIPv4 = regexp.MustCompile(`\b(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})\b`)
	// 手机号（大陆）
	rePhone = regexp.MustCompile(`\b1[3-9]\d{9}\b`)
	// 身份证 18 位
	reIDCard = regexp.MustCompile(`\b\d{17}[\dXx]\b`)
	// 邮箱
	reEmail = regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`)
)

// Apply 对文本执行脱敏；不修改原始存储，仅展示层。
func Apply(s string) string {
	// IPv4 白名单：127.0.0.1
	s = reIPv4.ReplaceAllStringFunc(s, func(m string) string {
		if m == "127.0.0.1" {
			return m
		}
		parts := strings.Split(m, ".")
		if len(parts) != 4 {
			return m
		}
		return "*.*." + parts[2] + "." + parts[3]
	})
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

// Writer 包装写接口，写出前脱敏。
type Writer interface {
	Write(p []byte) (int, error)
}

// MaskWriter 脱敏写包装。
type MaskWriter struct {
	W Writer
}

// Write 脱敏后写入。
func (m MaskWriter) Write(p []byte) (int, error) {
	out := Apply(string(p))
	n, err := m.W.Write([]byte(out))
	// 返回原始长度以符合 io.Writer 约定（可能被上游忽略差异）
	if err != nil {
		return n, err
	}
	return len(p), nil
}
