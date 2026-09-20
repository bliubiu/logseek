package stream

import (
	"context"
	"io"
)

// Opener 只读打开端口。
//
// 域内不触及文件系统实现（不再直接 os.Open），具体实现由基础设施层 fileio 提供，
// 由应用层装配注入；这样保证 domain 层对 infrastructure 零依赖。
type Opener interface {
	// Open 以只读语义打开 name；返回的句柄关闭由调用方负责。
	Open(name string) (io.ReadCloser, error)
}

// RateLimiter 顺序读限速端口。
//
// 具体实现由基础设施层 ratelimit 提供，应用层按需注入。
type RateLimiter interface {
	// Wait 消耗 n 字节配额，不足时阻塞等待至配额恢复或 ctx 取消。
	Wait(ctx context.Context, n int) error
	// Enabled 是否启用限速；未启用时调用方不做包装、零开销。
	Enabled() bool
}
