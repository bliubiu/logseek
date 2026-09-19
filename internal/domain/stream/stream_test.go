package stream_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bliubiu/logseek/internal/domain/stream"
	"github.com/bliubiu/logseek/internal/infrastructure/sink"
)

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
	sum, err := stream.Run(src, sk, []stream.LineFilter{matcher}, stream.DefaultOptions())
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
	opt := stream.Options{BlockSize: 64, MaxLine: 1 << 20}
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
	sum, err := stream.Run(src, sk, []stream.LineFilter{f1, f2}, stream.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if sum.Matched != 2 {
		t.Errorf("命中 = %d, 期望 2", sum.Matched)
	}
}

func TestMissingSource(t *testing.T) {
	_, err := stream.Run(filepath.Join(t.TempDir(), "no.log"), sink.NewDiscard(), nil, stream.DefaultOptions())
	if err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("期望不存在错误: %v", err)
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
