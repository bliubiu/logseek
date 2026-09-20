// Package fileio 提供只读文件打开与随机读适配。
package fileio

import (
	"fmt"
	"io"
	"os"

	"github.com/bliubiu/logseek/internal/domain/stream"
)

// Opener 实现 domain/stream 的只读打开端口。
type Opener struct{}

// NewOpener 创建读打开适配器。
func NewOpener() Opener { return Opener{} }

// 编译期断言：确保 Opener 始终满足域层端口。
var _ stream.Opener = Opener{}

// Open 以只读方式打开并返回句柄，满足 stream.Opener 端口。
func (Opener) Open(name string) (io.ReadCloser, error) {
	f, err := OpenReadOnly(name)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// ReadOnlyFile 只读文件句柄。
type ReadOnlyFile struct {
	f *os.File
}

// OpenReadOnly 以只读方式打开。
func OpenReadOnly(path string) (*ReadOnlyFile, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("文件不存在：%s", path)
		}
		if os.IsPermission(err) {
			return nil, fmt.Errorf("没有读取权限：%s", path)
		}
		return nil, fmt.Errorf("无法打开文件：%w", err)
	}
	return &ReadOnlyFile{f: f}, nil
}

// Read 顺序读（流式扫描场景复用文件读游标）。
func (r *ReadOnlyFile) Read(p []byte) (int, error) { return r.f.Read(p) }

// ReadAt 实现 io.ReaderAt。
func (r *ReadOnlyFile) ReadAt(p []byte, off int64) (int, error) {
	return r.f.ReadAt(p, off)
}

// Size 返回文件大小。
func (r *ReadOnlyFile) Size() (int64, error) {
	st, err := r.f.Stat()
	if err != nil {
		return 0, err
	}
	return st.Size(), nil
}

// Close 关闭句柄。
func (r *ReadOnlyFile) Close() error { return r.f.Close() }

// Path 返回路径。
func (r *ReadOnlyFile) Path() string { return r.f.Name() }
