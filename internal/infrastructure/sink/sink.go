// Package sink 实现结果文件写出（唯一写出口之一）。
package sink

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
)

// FileSink 带缓冲的文件写出器。
type FileSink struct {
	f    *os.File
	w    *bufio.Writer
	n    int64
	path string
	// mask 行级脱敏钩子；nil 表示原样写出。
	mask func(string) string
}

// Options 构造可选项。
type Options struct {
	// Mask 行级脱敏钩子，由应用层注入 infrastructure/mask.Apply；nil 表示不脱敏。
	Mask func(string) string
}

// New 创建输出文件（自动建父目录）；path 为空时返回丢弃型 sink。
func New(path string) (*FileSink, error) {
	return NewWithOptions(path, Options{})
}

// NewWithOptions 按选项创建输出文件；Mask 非空时每行写出前先脱敏。
func NewWithOptions(path string, o Options) (*FileSink, error) {
	if path == "" {
		return &FileSink{mask: o.Mask}, nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("创建输出目录失败：%w", err)
	}
	// 以创建+只写方式打开；存在则截断（结果文件语义）。
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, fmt.Errorf("创建输出文件失败：%w", err)
	}
	return &FileSink{
		f:    f,
		w:    bufio.NewWriterSize(f, 1<<20),
		path: path,
		mask: o.Mask,
	}, nil
}

// NewDiscard 丢弃写出（仅计数，stdout 场景）。
func NewDiscard() *FileSink { return &FileSink{} }

// WriteLine 写一行并追加换行。
//
// 脱敏按行施加而非按缓冲块：块级写出会把跨写边界的行切开，
// 造成半个 IP/手机号刚好落在边界而漏脱敏。
func (s *FileSink) WriteLine(line []byte) error {
	// 计数先行：丢弃型 sink 也要给出命中行数（策略场景需要）。
	s.n++
	if s.w == nil {
		return nil
	}
	if s.mask != nil {
		line = []byte(s.mask(string(line)))
	}
	if _, err := s.w.Write(line); err != nil {
		return err
	}
	if err := s.w.WriteByte('\n'); err != nil {
		return err
	}
	return nil
}

// Close 冲刷并关闭；幂等，重复调用返回 nil。
//
// 幂等是必要的：调用方可能同时存在 defer 兜底关闭与显式关闭两条路径，
// 二次关闭若不幂等会把成功流程误报为失败。
func (s *FileSink) Close() error {
	if s.w == nil {
		return nil
	}
	w, f := s.w, s.f
	s.w, s.f = nil, nil
	if err := w.Flush(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// Written 返回已写行数。
func (s *FileSink) Written() int64 { return s.n }

// Path 返回输出路径。
func (s *FileSink) Path() string { return s.path }
