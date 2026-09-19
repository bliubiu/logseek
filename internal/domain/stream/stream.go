// Package stream 提供只读分块流式扫描与过滤器组合。
package stream

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/bliubiu/logseek/internal/infrastructure/ratelimit"
	"github.com/bliubiu/logseek/internal/infrastructure/resources"
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

// Options 流式选项。
type Options struct {
	BlockSize int
	MaxLine   int
	// ReadRate 每秒字节上限，0 不限速
	ReadRate int64
	// Ctx 用于限速取消
	Ctx context.Context
}

// DefaultOptions 默认资源档位。
func DefaultOptions() Options {
	return Options{
		BlockSize: resources.DefaultBlockSize,
		MaxLine:   resources.MaxLineBytes,
	}
}

// Run 只读扫描 srcPath，依次应用 filters（与关系），命中写入 sink。
func Run(srcPath string, sink Sink, filters []LineFilter, opt Options) (Summary, error) {
	start := time.Now()
	if opt.BlockSize <= 0 {
		opt.BlockSize = resources.DefaultBlockSize
	}
	if opt.MaxLine <= 0 {
		opt.MaxLine = resources.MaxLineBytes
	}
	ctx := opt.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	lim := ratelimit.New(opt.ReadRate)
	sum := Summary{Source: srcPath}

	f, err := os.Open(srcPath) // 只读打开
	if err != nil {
		if os.IsNotExist(err) {
			return sum, fmt.Errorf("源文件不存在：%s", srcPath)
		}
		return sum, fmt.Errorf("无法打开源文件：%w", err)
	}
	defer f.Close()

	// 限速读包装
	var r io.Reader = f
	if lim.Enabled() {
		r = &rateReader{r: f, lim: lim, ctx: ctx}
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
			if err := sink.WriteLine(line); err != nil {
				sum.Duration = time.Since(start)
				return sum, fmt.Errorf("写出结果失败：%w", err)
			}
		}
	}
	if err := sc.Err(); err != nil {
		// 超长行等
		if err == bufio.ErrTooLong {
			sum.Duration = time.Since(start)
			return sum, fmt.Errorf("存在超过单行上限 %d 字节的日志行，已中止", opt.MaxLine)
		}
		sum.Duration = time.Since(start)
		return sum, fmt.Errorf("读取源文件失败：%w", err)
	}
	sum.Duration = time.Since(start)
	if sink != nil {
		if err := sink.Close(); err != nil {
			return sum, fmt.Errorf("关闭输出失败：%w", err)
		}
	}
	return sum, nil
}

// rateReader 按块限速读取。
type rateReader struct {
	r   io.Reader
	lim *ratelimit.Limiter
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

// 确保 io.EOF 引用不被误删（Scanner 内部处理 EOF）。
var _ = io.EOF
