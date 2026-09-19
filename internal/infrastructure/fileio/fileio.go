// Package fileio 提供只读文件打开与随机读适配。
package fileio

import (
	"fmt"
	"os"
)

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
