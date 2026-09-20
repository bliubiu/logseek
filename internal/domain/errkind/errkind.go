// Package errkind 定义错误类别，供 CLI 映射稳定退出码。
//
// 背景：早期实现依赖错误文本子串（含中文）判断退出码，
// 一旦文案调整或包装层级变化就会误判。这里用带类别的错误类型承载语义，
// 调用方只需 errors.As 或 KindOf 即可，不再解析文本。
package errkind

import (
	"errors"
	"fmt"
	"strings"
)

// Kind 错误类别。
type Kind int

const (
	// KindUnknown 未分类错误，默认按运行错误处理。
	KindUnknown Kind = iota
	// KindUsage 命令行用法错误（参数缺失或互斥）。
	KindUsage
	// KindNotFound 源文件或资源不存在、无读取权限。
	KindNotFound
	// KindProbeFailed 预检失败：时间格式无法识别、布局解析失败、文件为空等。
	KindProbeFailed
	// KindRuntime 运行时错误：写盘失败、密钥初始化失败等。
	KindRuntime
	// KindInterrupt 被上下文取消或中断。
	KindInterrupt
)

// String 便于日志与调试输出。
func (k Kind) String() string {
	switch k {
	case KindUsage:
		return "usage"
	case KindNotFound:
		return "not-found"
	case KindProbeFailed:
		return "probe-failed"
	case KindRuntime:
		return "runtime"
	case KindInterrupt:
		return "interrupt"
	default:
		return "unknown"
	}
}

// Error 带类别的错误。
type Error struct {
	Kind Kind
	Msg  string
	Err  error
}

// New 创建带类别的错误；Msg 使用中文，格式同 fmt.Errorf。
func New(kind Kind, format string, args ...any) *Error {
	return &Error{Kind: kind, Msg: fmt.Sprintf(format, args...)}
}

// Wrap 保留下层错误文案与调用链（errors.Is/As 可用），并标注类别。
func Wrap(kind Kind, err error) error {
	if err == nil {
		return nil
	}
	if e, ok := err.(*Error); ok {
		// 已分类则以最具体的那一层为准
		if e.Kind == KindUnknown && kind != KindUnknown {
			e.Kind = kind
		}
		return e
	}
	return &Error{Kind: kind, Msg: err.Error(), Err: err}
}

// Error 实现 error。
func (e *Error) Error() string {
	if e.Err != nil && e.Msg != e.Err.Error() && !strings.Contains(e.Msg, e.Err.Error()) {
		return e.Msg + "：" + e.Err.Error()
	}
	return e.Msg
}

// Unwrap 返回被包装的下层错误。
func (e *Error) Unwrap() error { return e.Err }

// KindOf 提取错误链上的类别；取不到返回 KindUnknown。
func KindOf(err error) Kind {
	if err == nil {
		return KindUnknown
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return KindUnknown
}
