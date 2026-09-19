// Package resources 提供生产共存相关资源约束接线。
package resources

import (
	"runtime"
	"runtime/debug"
)

// 默认软内存上限（字节），对应架构红线 R-MEM2：GOMEMLIMIT ≈ 192MiB。
const DefaultMemoryLimitBytes = 192 << 20

// 默认读块大小（字节），对应红线 R-BLK：1MiB。
const DefaultBlockSize = 1 << 20

// 单行长度上限（字节），对应红线 R-LINE：1MiB。
const MaxLineBytes = 1 << 20

// 同时打开文件句柄上限，对应红线 R-FD。
const MaxOpenFiles = 8

// ApplyMemoryLimit 启用 Go 运行时软内存上限；limit<=0 时使用默认值。
// 返回实际生效的上限，便于测试断言「默认非关闭」。
func ApplyMemoryLimit(limit int64) int64 {
	if limit <= 0 {
		limit = DefaultMemoryLimitBytes
	}
	return debug.SetMemoryLimit(limit)
}

// CurrentMemoryLimit 返回当前软内存上限（调试/摘要用）。
func CurrentMemoryLimit() int64 {
	return debug.SetMemoryLimit(-1)
}

// SetMaxProcs 返回当前并行度提示；首期顺序管线不主动抬高并行度。
func GOMAXPROCS() int {
	return runtime.GOMAXPROCS(0)
}
