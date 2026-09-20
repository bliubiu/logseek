// Package stream 提供只读分块流式扫描与过滤器组合。
package stream

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"time"
)

// LineFilter 行级过滤器；返回是否写出。
type LineFilter interface {
	Accept(line []byte) bool
}

// FilterFunc 函数适配。
type FilterFunc func(line []byte) bool

// Accept 实现 LineFilter。
func (f FilterFunc) Accept(line []byte) bool { return f(line) }

// Sink 结果写出。
type Sink interface {
	WriteLine(line []byte) error
	Close() error
}

// Summary 执行摘要。
type Summary struct {
	Scanned  int64         `json:"scanned"`
	Matched  int64         `json:"matched"`
	BadLines int64         `json:"bad_lines,omitempty"`
	Duration time.Duration `json:"duration_ms"`
	Source   string        `json:"source"`
	Output   string        `json:"output,omitempty"`
}

// Format 中文摘要。
func (s Summary) Format() string {
	return fmt.Sprintf("扫描行: %d；命中行: %d；坏行: %d；耗时: %s",
		s.Scanned, s.Matched, s.BadLines, s.Duration.Round(time.Millisecond))
}

// JSON 输出 JSON 字段友好结构。
func (s Summary) JSON() map[string]any {
	return map[string]any{
		"scanned":     s.Scanned,
		"matched":     s.Matched,
		"bad_lines":   s.BadLines,
		"duration_ms": s.Duration.Milliseconds(),
		"source":      s.Source,
		"output":      s.Output,
	}
}

// 域级缺省档位：应用层应以 infrastructure/resources 的生产红线为准显式注入，
// 此处仅用于未注入时的兜底，避免把基础设施常量反向引入域层。
const (
	// DefaultBlockSize 默认读块大小（1MiB）。
	DefaultBlockSize = 1 << 20
	// DefaultMaxLine 默认单行上限（1MiB）。
	DefaultMaxLine = 1 << 20
)

// Options 流式选项。
type Options struct {
	BlockSize int
	MaxLine   int
	// Opener 只读打开端口，必填；未注入时返回错误而非回退到 os.Open。
	Opener Opener
	// Limiter 限速端口，nil 表示不限速。
	Limiter RateLimiter
	// Ctx 用于限速等待的取消。
	Ctx context.Context
}

// DefaultOptions 默认资源档位；仅含块与行上限，端口由调用方注入。
func DefaultOptions() Options {
	return Options{
		BlockSize: DefaultBlockSize,
		MaxLine:   DefaultMaxLine,
	}
}

// Run 只读扫描 srcPath，依次应用 filters（与关系），命中写入 sink。
func Run(srcPath string, sink Sink, filters []LineFilter, opt Options) (sum Summary, err error) {
	start := time.Now()
	if opt.BlockSize <= 0 {
		opt.BlockSize = DefaultBlockSize
	}
	if opt.MaxLine <= 0 {
		opt.MaxLine = DefaultMaxLine
	}
	ctx := opt.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	sum = Summary{Source: srcPath}

	if opt.Opener == nil {
		return sum, fmt.Errorf("缺少文件打开端口：调用方需注入 infrastructure/fileio 提供的实现")
	}
	f, oerr := opt.Opener.Open(srcPath)
	if oerr != nil {
		return sum, oerr
	}
	defer f.Close()

	// 写出端口必须在成功与出错两种路径上都关闭：
	// 否则错误返回时缓冲内容丢失、文件句柄泄漏（结果文件残缺）。
	if sink != nil {
		defer func() {
			if cerr := sink.Close(); cerr != nil && err == nil {
				err = fmt.Errorf("关闭输出失败：%w", cerr)
			}
		}()
	}

	// 限速读包装
	var r io.Reader = f
	if opt.Limiter != nil && opt.Limiter.Enabled() {
		r = &rateReader{r: f, lim: opt.Limiter, ctx: ctx}
	}

	// bufio 大缓冲顺序读，避免随机抖动。
	br := bufio.NewReaderSize(r, opt.BlockSize)
	sc := bufio.NewScanner(br)
	sc.Buffer(make([]byte, 0, 64<<10), opt.MaxLine)

	for sc.Scan() {
		line := sc.Bytes()
		sum.Scanned++
		ok := true
		for _, flt := range filters {
			if !flt.Accept(line) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		sum.Matched++
		if sink != nil {
			if werr := sink.WriteLine(line); werr != nil {
				sum.Duration = time.Since(start)
				return sum, fmt.Errorf("写出结果失败：%w", werr)
			}
		}
	}
	if serr := sc.Err(); serr != nil {
		sum.Duration = time.Since(start)
		// 超长行等
		if serr == bufio.ErrTooLong {
			return sum, fmt.Errorf("存在超过单行上限 %d 字节的日志行，已中止", opt.MaxLine)
		}
		return sum, fmt.Errorf("读取源文件失败：%w", serr)
	}
	sum.Duration = time.Since(start)
	return sum, nil
}

// rateReader 按块限速读取。
type rateReader struct {
	r   io.Reader
	lim RateLimiter
	ctx context.Context
}

func (rr *rateReader) Read(p []byte) (int, error) {
	n, err := rr.r.Read(p)
	if n > 0 {
		if werr := rr.lim.Wait(rr.ctx, n); werr != nil {
			return n, werr
		}
	}
	return n, err
}
