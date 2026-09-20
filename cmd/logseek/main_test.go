package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
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

func TestCLIExitCodesByKind(t *testing.T) {
	bin := buildBinary(t)
	src, dir := writeFixture(t)
	logDir := filepath.Join(dir, "logs")

	cases := []struct {
		name string
		args []string
		want int
	}{
		{"源文件不存在→3", []string{"inspect", filepath.Join(dir, "nope.log"), "--log-dir", logDir}, 3},
		{"缺少检索条件→2", []string{"grep", src, "--log-dir", logDir}, 2},
		{"slice 缺 --start/--end→2", []string{"slice", src, "--log-dir", logDir}, 2},
		{"export 无条件→2", []string{"export", src, "--log-dir", logDir}, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cmd := exec.Command(bin, append(c.args, "--log-dir", logDir)...)
			if code := exitCode(t, cmd); code != c.want {
				t.Fatalf("退出码 = %d, 期望 %d", code, c.want)
			}
		})
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

// repoFile 返回仓库根 testdata 下文件的绝对路径（测试工作目录为 cmd/logseek）。
func repoFile(t *testing.T, name string) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("缺少夹具 %s: %v", p, err)
	}
	return p
}

func sha256file(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%x", sum)
}

// 端到端：以真实 Oracle alert 日志采样（testdata/alert_dlscdb_sample.log）验证
// 预检/切片/检索/组合导出与源文件只读铁律。
func TestCLIAlertE2E(t *testing.T) {
	bin := buildBinary(t)
	src := repoFile(t, "alert_dlscdb_sample.log")
	dir := t.TempDir()
	logDir := filepath.Join(dir, "logs")
	sumBefore := sha256file(t, src)

	// 1) inspect：Oracle alert 格式识别，年份完整
	cmd := exec.Command(bin, "inspect", src, "--json", "--log-dir", logDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect 失败: %v\n%s", err, out)
	}
	for _, want := range []string{
		`"time_layout": "Mon Jan 02 15:04:05 2006"`,
		`"time_detected": true`,
		`"sliceable": true`,
		`"first_time": "2019-03-16 16:11:32"`,
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("inspect 输出缺少 %s:\n%s", want, out)
		}
	}

	// 2) slice：2019-03-16 窗口命中启动时间戳行；内容行为坏行计数
	cmd = exec.Command(bin, "slice", src,
		"--start", "2019-03-16 00:00:00", "--end", "2019-03-17 00:00:00",
		"--json", "--log-dir", logDir)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("slice 失败: %v\n%s", err, out)
	}
	for _, want := range []string{`"matched":1`, `"scanned":94`, `"bad_lines":89`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("slice 输出缺少 %s:\n%s", want, out)
		}
	}

	// 3) slice 2026-09-20 窗口命中 4 条跨日时间戳行
	cmd = exec.Command(bin, "slice", src,
		"--start", "2026-09-20 00:00:00", "--end", "2026-09-21 00:00:00",
		"-o", filepath.Join(dir, "sl.log"), "--json", "--log-dir", logDir)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("slice 失败: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), `"matched":4`) {
		t.Errorf("slice 2026 期望命中 4:\n%s", out)
	}

	// 4) grep：检索 LOGMINER 命中 62 行
	cmd = exec.Command(bin, "grep", src, "-k", "LOGMINER", "--json", "--log-dir", logDir)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("grep 失败: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), `"matched":62`) {
		t.Errorf("grep 期望命中 62: %s", out)
	}

	// 5) export 组合：2026 窗口 ∧ LOGMINER -> 内容行无行首时间戳，交集为空
	cmd = exec.Command(bin, "export", src,
		"--start", "2026-09-20 00:00:00", "--end", "2026-09-21 00:00:00",
		"-k", "LOGMINER", "--json", "--log-dir", logDir)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("export 失败: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), `"matched":0`) {
		t.Errorf("export 组合期望交集为空: %s", out)
	}

	// 6) export 纯内容：无时间条件仍可检索（采样尾部含 ORA-12012 错误块）
	cmd = exec.Command(bin, "export", src, "-k", "ORA-12012", "--json", "--log-dir", logDir)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("export 纯内容失败: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), `"matched":1`) {
		t.Errorf("ORA-12012 应命中采样错误块: %s", out)
	}
	// 但纯内容检索 ORA- 前缀应命中错误块行（采样本不含 ORA，用 LOGMINER 验证命中）
	cmd = exec.Command(bin, "export", src, "-k", "LOGMINER", "-o", filepath.Join(dir, "g.log"), "--json", "--log-dir", logDir)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("export 检索失败: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), `"matched":62`) {
		t.Errorf("export 检索期望命中 62: %s", out)
	}

	// 7) 安全铁律：全程源文件只读，校验和不得变化
	if got := sha256file(t, src); got != sumBefore {
		t.Errorf("源文件被修改！before=%s after=%s", sumBefore, got)
	}
}

func TestCLISaveConfigEncryptsPassword(t *testing.T) {
	bin := buildBinary(t)
	src, dir := writeFixture(t)
	keyPath := filepath.Join(dir, "key")
	cfgPath := filepath.Join(dir, "cfg.json")
	logDir := filepath.Join(dir, "logs")

	cmd := exec.Command(bin, "inspect", src,
		"--save-config", cfgPath, "--ensure-password",
		"--key-file", keyPath, "--log-dir", logDir)
	if code := exitCode(t, cmd); code != 0 {
		t.Fatalf("退出码 = %d, 期望 0", code)
	}
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	// 口令必须密文落盘
	if !strings.Contains(string(raw), "ENC(") {
		t.Fatalf("口令未加密落盘: %s", raw)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if pwd, _ := decoded["password"].(string); !strings.HasPrefix(pwd, "ENC(") {
		t.Fatalf("password 字段非 ENC: %v", decoded["password"])
	}
	// 密钥文件不应留在配置文件里，回读应成功
	cmd = exec.Command(bin, "inspect", src, "--config", cfgPath, "--key-file", keyPath, "--log-dir", logDir)
	if code := exitCode(t, cmd); code != 0 {
		t.Fatalf("带配置回读退出码 = %d, 期望 0", code)
	}
}
