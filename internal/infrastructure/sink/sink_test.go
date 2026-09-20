package sink_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bliubiu/logseek/internal/infrastructure/sink"
)

func outPath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "out", name)
}

func TestWriteLineAndFlush(t *testing.T) {
	p := outPath(t, "r.log")
	sk, err := sink.New(p)
	if err != nil {
		t.Fatal(err)
	}
	// 尚未 Close 时不应落盘（缓冲未刷）
	for _, l := range []string{"line-1", "line-2"} {
		if err := sk.WriteLine([]byte(l)); err != nil {
			t.Fatal(err)
		}
	}
	if sk.Written() != 2 {
		t.Fatalf("Written = %d, 期望 2", sk.Written())
	}
	if sk.Path() != p {
		t.Fatalf("Path = %s, 期望 %s", sk.Path(), p)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "line-1") {
		t.Fatalf("未 Close 即已落盘，缓冲语义异常: %s", b)
	}
	if err := sk.Close(); err != nil {
		t.Fatal(err)
	}
	b, err = os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), "line-1\nline-2\n"; got != want {
		t.Fatalf("写出内容 = %q, 期望 %q", got, want)
	}
}

func TestRepeatedCloseIsSafe(t *testing.T) {
	sk, err := sink.New(outPath(t, "c.log"))
	if err != nil {
		t.Fatal(err)
	}
	if err := sk.Close(); err != nil {
		t.Fatal(err)
	}
	// 重复关闭必须幂等，否则上层 defer + 显式关闭的组合会误报失败
	if err := sk.Close(); err != nil {
		t.Fatalf("重复 Close 报错: %v", err)
	}
}

func TestMaskHookAppliedPerLine(t *testing.T) {
	p := outPath(t, "m.log")
	sk, err := sink.NewWithOptions(p, sink.Options{Mask: func(s string) string {
		return strings.ReplaceAll(s, "secret", "***")
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := sk.WriteLine([]byte("has secret data")); err != nil {
		t.Fatal(err)
	}
	if err := sk.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "secret") {
		t.Fatalf("脱敏钩子未生效: %s", b)
	}
	if !strings.Contains(string(b), "***") {
		t.Fatalf("脱敏占位缺失: %s", b)
	}
}

func TestDiscardSinkNoops(t *testing.T) {
	sk := sink.NewDiscard()
	for i := 0; i < 3; i++ {
		if err := sk.WriteLine([]byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if err := sk.Close(); err != nil {
		t.Fatal(err)
	}
	if sk.Written() != 3 {
		t.Fatalf("丢弃型仍应计数，实际 %d", sk.Written())
	}
	if _, err := os.Stat(sk.Path()); err == nil {
		t.Fatal("丢弃型不应产生真实文件")
	}
}

func TestEmptyPathReturnsDiscard(t *testing.T) {
	sk, err := sink.New("")
	if err != nil {
		t.Fatal(err)
	}
	if err := sk.WriteLine([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := sk.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateOutputDirFailed(t *testing.T) {
	// 用一个已存在的普通文件充当父目录，制造创建失败
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := sink.New(filepath.Join(blocker, "sub", "out.log"))
	if err == nil {
		t.Fatal("期望创建输出目录失败")
	}
	if !strings.Contains(err.Error(), "创建输出目录失败") {
		t.Fatalf("错误信息不友好: %v", err)
	}
}

func TestResultFileTruncated(t *testing.T) {
	// 结果文件语义：重复导出必须截断旧内容而非追加
	p := outPath(t, "t.log")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("旧内容旧内容旧内容\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sk, err := sink.New(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := sk.WriteLine([]byte("新内容")); err != nil {
		t.Fatal(err)
	}
	if err := sk.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "新内容\n" {
		t.Fatalf("结果文件未截断: %q", b)
	}
}
