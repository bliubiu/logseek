package logging_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bliubiu/logseek/internal/infrastructure/logging"
)

func TestWriteAndFormat(t *testing.T) {
	dir := t.TempDir()
	lg, err := logging.New(logging.Options{
		Level:      "INFO",
		Dir:        dir,
		Console:    false,
		Module:     "test",
		EnableMask: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer lg.Close()
	lg.Infof("处理完成 手机13812345678")

	files := logging.LogFiles(dir)
	if len(files) != 1 {
		t.Fatalf("files=%v", files)
	}
	day := timeNowDay()
	if files[0] != "logseek-"+day+".log" {
		t.Fatalf("文件名=%s", files[0])
	}
	b, _ := os.ReadFile(filepath.Join(dir, files[0]))
	s := string(b)
	if !strings.Contains(s, "[INFO]") || !strings.Contains(s, "[test]") {
		t.Fatalf("格式错误: %s", s)
	}
	if !strings.Contains(s, "[20") {
		t.Fatalf("缺少时间戳: %s", s)
	}
	if strings.Contains(s, "13812345678") {
		t.Fatal("日志中手机号未脱敏")
	}
}

func TestLevelFilter(t *testing.T) {
	dir := t.TempDir()
	lg, _ := logging.New(logging.Options{Level: "ERROR", Dir: dir})
	defer lg.Close()
	lg.Infof("应被过滤")
	lg.Errorf("应记录")
	b, _ := os.ReadFile(filepath.Join(dir, logging.LogFiles(dir)[0]))
	s := string(b)
	if strings.Contains(s, "应被过滤") {
		t.Fatal("INFO 不应写入 ERROR 级")
	}
	if !strings.Contains(s, "应记录") {
		t.Fatal("ERROR 应写入")
	}
}

func TestCleanupOld(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "logseek-20000101.log")
	_ = os.WriteFile(old, []byte("x"), 0o644)
	lg, err := logging.New(logging.Options{Level: "INFO", Dir: dir, Retention: 32})
	if err != nil {
		t.Fatal(err)
	}
	defer lg.Close()
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("过期日志应清理")
	}
}

func timeNowDay() string {
	return timeFormatDay()
}
