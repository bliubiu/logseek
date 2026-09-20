package stream_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bliubiu/logseek/internal/domain/stream"
	"github.com/bliubiu/logseek/internal/infrastructure/sink"
)

// fileOpener 测试用只读打开端口实现（不引入基础设施，保持域层测试纯净）。
type fileOpener struct{}

func (fileOpener) Open(name string) (io.ReadCloser, error) {
	f, err := os.Open(name)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("文件不存在：%s", name)
		}
		return nil, fmt.Errorf("无法打开源文件：%w", err)
	}
	return f, nil
}

// slowLimiter 按每字节批次阻塞的限速端口实现，用于验证端口确实被接线。
type slowLimiter struct {
	calls  int64
	waited time.Duration
	every  time.Duration
}

func (s *slowLimiter) Wait(_ context.Context, n int) error {
	s.calls++
	time.Sleep(s.every)
	s.waited += s.every
	return nil
}

func (s *slowLimiter) Enabled() bool { return true }

// optsWithSrc 组装带测试打开器的默认选项。
func optsWithSrc() stream.Options {
	opt := stream.DefaultOptions()
	opt.Opener = fileOpener{}
	return opt
}

func writeLog(t *testing.T, lines []string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "src.log")
	body := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func fileSHA(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestReadOnlyAndOrder(t *testing.T) {
	lines := []string{
		"2026-01-01 08:00:00.000 line-1",
		"2026-01-01 08:00:01.000 line-2",
		"2026-01-01 08:00:02.000 line-3 ERROR",
	}
	src := writeLog(t, lines)
	before := fileSHA(t, src)

	out := filepath.Join(t.TempDir(), "out.log")
	sk, err := sink.New(out)
	if err != nil {
		t.Fatal(err)
	}
	matcher := stream.FilterFunc(func(line []byte) bool {
		return strings.Contains(string(line), "line")
	})
	sum, err := stream.Run(src, sk, []stream.LineFilter{matcher}, optsWithSrc())
	if err != nil {
		t.Fatal(err)
	}
	if sum.Scanned != 3 || sum.Matched != 3 {
		t.Errorf("摘要扫描/命中 = %d/%d", sum.Scanned, sum.Matched)
	}
	after := fileSHA(t, src)
	if before != after {
		t.Fatal("源文件校验和变化，违反只读")
	}
	got, _ := os.ReadFile(out)
	if strings.TrimSpace(string(got)) != strings.Join(lines, "\n") {
		t.Errorf("输出顺序/内容不一致:\n%s", got)
	}
}

func TestChunkBoundaryLines(t *testing.T) {
	// 构造跨缓冲边界的行
	var lines []string
	for i := 0; i < 1000; i++ {
		lines = append(lines, "行内容填充填充填充填充填充填充填充-"+strings.Repeat("x", 50)+"-"+itoa(i))
	}
	src := writeLog(t, lines)
	sk := sink.NewDiscard()
	opt := optsWithSrc()
	opt.BlockSize = 64
	sum, err := stream.Run(src, sk, nil, opt)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Scanned != 1000 {
		t.Errorf("扫描行 = %d, 期望 1000", sum.Scanned)
	}
}

func TestFilterAndComposition(t *testing.T) {
	src := writeLog(t, []string{
		"2026-01-01 10:00:00 ERROR a",
		"2026-01-01 10:00:01 INFO b",
		"2026-01-01 10:00:02 ERROR c",
	})
	sk := sink.NewDiscard()
	// 时间过滤用函数近似：只保留 10:00:00 与 10:00:02 的 ERROR —— 组合为与
	f1 := stream.FilterFunc(func(b []byte) bool { return strings.Contains(string(b), "ERROR") })
	f2 := stream.FilterFunc(func(b []byte) bool { return !strings.Contains(string(b), "INFO") })
	sum, err := stream.Run(src, sk, []stream.LineFilter{f1, f2}, optsWithSrc())
	if err != nil {
		t.Fatal(err)
	}
	if sum.Matched != 2 {
		t.Errorf("命中 = %d, 期望 2", sum.Matched)
	}
}

func TestMissingSource(t *testing.T) {
	_, err := stream.Run(filepath.Join(t.TempDir(), "no.log"), sink.NewDiscard(), nil, optsWithSrc())
	if err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("期望不存在错误: %v", err)
	}
}

func TestReadRate(t *testing.T) {
	// 低速端口：验证 RateLimiter 端口确实被调用（不再由域层直接 new 基础设施）
	var lines []string
	for i := 0; i < 8; i++ {
		lines = append(lines, strings.Repeat("y", 400)+itoa(i))
	}
	src := writeLog(t, lines)
	sk := sink.NewDiscard()
	// 每次底层读都会经过限速端口；读粒度由 Scanner 缓冲决定（绕过 BlockSize），
	// 因此这里只断言端口被调用且阻塞确实累积，不断言调用次数。
	metric := &slowLimiter{every: 60 * time.Millisecond}
	opt := optsWithSrc()
	opt.Limiter = metric
	start := time.Now()
	sum, err := stream.Run(src, sk, nil, opt)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Scanned != 8 {
		t.Fatalf("scanned=%d", sum.Scanned)
	}
	if metric.calls == 0 {
		t.Fatal("限速端口未被调用")
	}
	if time.Since(start) < 50*time.Millisecond {
		t.Fatalf("限速似乎未生效: %s", time.Since(start))
	}
}

func TestMissingOpener(t *testing.T) {
	// 未注入打开端口必须报错，而不是悄悄回退到 os.Open
	_, err := stream.Run("x.log", sink.NewDiscard(), nil, stream.DefaultOptions())
	if err == nil || !strings.Contains(err.Error(), "文件打开端口") {
		t.Fatalf("期望缺少端口错误: %v", err)
	}
}

// failSink 写第 3 行失败，用于验证错误路径同样会 Close。
type failSink struct {
	written int
	closed  bool
}

func (f *failSink) WriteLine(_ []byte) error {
	f.written++
	if f.written >= 3 {
		return fmt.Errorf("模拟写盘失败")
	}
	return nil
}

func (f *failSink) Close() error {
	f.closed = true
	return nil
}

func TestSinkClosedOnErrorPath(t *testing.T) {
	// 回归：错误返回路径也必须关闭写出，否则缓冲丢失 + 句柄泄漏
	src := writeLog(t, []string{"a", "b", "c", "d", "e"})
	sk := &failSink{}
	if _, err := stream.Run(src, sk, nil, optsWithSrc()); err == nil {
		t.Fatal("期望写出失败")
	}
	if !sk.closed {
		t.Fatal("错误路径未关闭 sink，存在句柄泄漏与缓冲丢失风险")
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
