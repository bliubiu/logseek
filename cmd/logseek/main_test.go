package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// 端到端：构建二进制后执行主路径。
func buildBinary(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "logseek.exe")
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = "."
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("构建失败: %v\n%s", err, b)
	}
	return out
}

func writeFixture(t *testing.T) (src, dir string) {
	t.Helper()
	dir = t.TempDir()
	src = filepath.Join(dir, "app.log")
	body := strings.Join([]string{
		"2026-01-01 08:00:00.000 [INFO] 启动",
		"2026-01-01 08:00:01.000 [ERROR] 失败 手机13812345678",
		"2026-01-01 08:30:00.000 [ERROR] 再次",
	}, "\n") + "\n"
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return src, dir
}

func exitCode(t *testing.T, cmd *exec.Cmd) int {
	t.Helper()
	err := cmd.Run()
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	t.Fatalf("执行失败: %v", err)
	return -1
}

func TestCLIInspectAndGrepExitCodes(t *testing.T) {
	bin := buildBinary(t)
	src, dir := writeFixture(t)
	logDir := filepath.Join(dir, "logs")

	// inspect 成功
	cmd := exec.Command(bin, "inspect", src, "--log-dir", logDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect 失败: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "预检报告") {
		t.Fatalf("输出: %s", out)
	}

	// grep 输出
	outPath := filepath.Join(dir, "g.log")
	cmd = exec.Command(bin, "grep", src, "-k", "ERROR", "-o", outPath, "--log-dir", logDir)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("grep 失败: %v\n%s", err, out)
	}
	got, _ := os.ReadFile(outPath)
	if !strings.Contains(string(got), "失败") {
		t.Fatalf("grep 输出: %s", got)
	}

	// 不存在文件退出码 3
	cmd = exec.Command(bin, "inspect", filepath.Join(dir, "nope.log"), "--log-dir", logDir)
	if code := exitCode(t, cmd); code != 3 {
		t.Fatalf("期望退出码 3, got %d", code)
	}

	// 参数错误退出码 2（grep 无关键词）
	cmd = exec.Command(bin, "grep", src, "--log-dir", logDir)
	if code := exitCode(t, cmd); code != 2 {
		t.Fatalf("期望退出码 2, got %d", code)
	}
}

func TestCLIExportJSON(t *testing.T) {
	bin := buildBinary(t)
	src, dir := writeFixture(t)
	cmd := exec.Command(bin, "export", src,
		"--start", "2026-01-01 08:00:00",
		"--end", "2026-01-01 09:00:00",
		"-k", "ERROR", "--json",
		"--log-dir", filepath.Join(dir, "logs"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(string(out), `"matched"`) {
		t.Fatalf("JSON 摘要缺失: %s", out)
	}
}
