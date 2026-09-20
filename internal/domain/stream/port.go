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

// RandomReader 支持随机读的只读句柄。
//
// 用于在已定位的字节区间上做受限顺序扫描，避免为极小的时间窗口全量扫描大文件。
type RandomReader interface {
	io.ReaderAt
	// Size 返回总字节数。
	Size() (int64, error)
	// Close 释放句柄；由 Run 负责调用。
	Close() error
}

// RandomOpener 随机读打开端口；可选注入。
//
// 仅在 Options 给出了有效扫描区间时使用；未注入时 Run 退化为全文件顺序扫描，
// 语义不变（只是无法裁剪读取量）。
type RandomOpener interface {
	// OpenRandom 以只读语义打开 name；返回的句柄关闭由调用方负责。
	OpenRandom(name string) (RandomReader, error)
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
