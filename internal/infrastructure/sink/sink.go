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
	f  *os.File
	w  *bufio.Writer
	n  int64
	path string
}

// New 创建输出文件（自动建父目录）；path 为空时返回丢弃型 sink。
func New(path string) (*FileSink, error) {
	if path == "" {
		return &FileSink{}, nil
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
	}, nil
}

// NewDiscard 丢弃写出（仅计数，stdout 场景）。
func NewDiscard() *FileSink { return &FileSink{} }

// WriteLine 写一行并追加换行。
func (s *FileSink) WriteLine(line []byte) error {
	if s.w == nil {
		return nil
	}
	if _, err := s.w.Write(line); err != nil {
		return err
	}
	if err := s.w.WriteByte('\n'); err != nil {
		return err
	}
	s.n++
	return nil
}

// Close 冲刷并关闭。
func (s *FileSink) Close() error {
	if s.w == nil {
		return nil
	}
	if err := s.w.Flush(); err != nil {
		_ = s.f.Close()
		return err
	}
	return s.f.Close()
}

// Written 返回已写行数。
func (s *FileSink) Written() int64 { return s.n }

// Path 返回输出路径。
func (s *FileSink) Path() string { return s.path }
