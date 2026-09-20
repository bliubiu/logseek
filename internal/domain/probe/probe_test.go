package probe_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bliubiu/logseek/internal/domain/probe"
	"github.com/bliubiu/logseek/internal/infrastructure/fileio"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDetectUTF8WithTimestamp(t *testing.T) {
	p := writeTemp(t, strings.Join([]string{
		"2026-01-01 08:00:00.123 [INFO] 服务启动",
		"2026-01-01 08:00:01.456 [ERROR] 连接失败",
		"2026-01-01 09:00:00.000 [INFO] 恢复正常",
	}, "\n")+"\n")
	rf, err := fileio.OpenReadOnly(p)
	if err != nil {
		t.Fatal(err)
	}
	defer rf.Close()

	rep, err := probe.Detect(rf)
	if err != nil {
		t.Fatalf("预检失败: %v", err)
	}
	if rep.Encoding != "utf-8" {
		t.Errorf("编码 = %q, 期望 utf-8", rep.Encoding)
	}
	if !rep.TimeDetected || !rep.Sliceable {
		t.Errorf("应识别到时间格式: %+v", rep)
	}
	if rep.FirstTime == "" || rep.LastTime == "" {
		t.Errorf("应包含首末时间: %+v", rep)
	}
	if !strings.Contains(rep.Format(), "预检报告") {
		t.Errorf("格式化输出异常")
	}
}

func TestDetectEmptyFile(t *testing.T) {
	p := writeTemp(t, "")
	rf, _ := fileio.OpenReadOnly(p)
	defer rf.Close()
	_, err := probe.Detect(rf)
	if err == nil {
		t.Fatal("空文件应报错")
	}
	if !strings.Contains(err.Error(), "空") {
		t.Errorf("错误信息应为中文且含空: %v", err)
	}
}

func TestDetectNoTimestamp(t *testing.T) {
	p := writeTemp(t, "hello\nworld\n")
	rf, _ := fileio.OpenReadOnly(p)
	defer rf.Close()
	rep, err := probe.Detect(rf)
	if err != nil {
		t.Fatal(err)
	}
	if rep.TimeDetected {
		t.Error("无时间戳不应检测为可切片")
	}
	if rep.Hint == "" {
		t.Error("应给出中文提示")
	}
}

func TestMissingFile(t *testing.T) {
	_, err := fileio.OpenReadOnly(filepath.Join(t.TempDir(), "nope.log"))
	if err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("期望中文不存在错误, got %v", err)
	}
}

// 回归：Oracle alert 日志（例如 testdata/alert_dlscdb1.log）预检必须
// 识别带年份完整布局，首末时间年份不得丢失为 0000。
func TestDetectOracleAlertFormat(t *testing.T) {
	p := writeTemp(t, strings.Join([]string{
		"Sat Mar 16 16:11:32 2019",
		"Starting ORACLE instance (normal)",
		"LOGMINER: Begin mining logfile for session -2147264255 thread 1",
		"Sun Sep 20 09:15:00 2026",
	}, "\n")+"\n")
	rf, err := fileio.OpenReadOnly(p)
	if err != nil {
		t.Fatal(err)
	}
	defer rf.Close()

	rep, err := probe.Detect(rf)
	if err != nil {
		t.Fatalf("预检失败: %v", err)
	}
	if !rep.TimeDetected || !rep.Sliceable {
		t.Fatalf("应识别到时间格式: %+v", rep)
	}
	if rep.TimeLayout != "Mon Jan 02 15:04:05 2006" {
		t.Fatalf("时间布局 = %q", rep.TimeLayout)
	}
	if !strings.HasPrefix(rep.FirstTime, "2019") {
		t.Errorf("首条时间年份丢失: %q", rep.FirstTime)
	}
	if !strings.HasPrefix(rep.LastTime, "2026") {
		t.Errorf("末条时间年份丢失: %q", rep.LastTime)
	}
}

// 回归：尾部样例必须取自文件真实末尾。
// 旧实现对尾部窗口取「前 15 行中的后 5 行」，导致展示的只是窗口开头的内容。
func TestDetectTailSamplesFromEOF(t *testing.T) {
	var lines []string
	for i := 1; i <= 20; i++ {
		lines = append(lines, fmt.Sprintf("2026-01-01 08:%02d:00 line%d", i, i))
	}
	p := writeTemp(t, strings.Join(lines, "\n")+"\n")
	rf, err := fileio.OpenReadOnly(p)
	if err != nil {
		t.Fatal(err)
	}
	defer rf.Close()

	rep, err := probe.Detect(rf)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.TailSamples) == 0 {
		t.Fatal("尾部样例为空")
	}
	if got := rep.TailSamples[len(rep.TailSamples)-1]; got != "2026-01-01 08:20:00 line20" {
		t.Errorf("末条尾部样例 = %q，期望 line20", got)
	}
}

// 回归：日志末尾为多行续写（无时间戳）时，末条时间仍须回溯到最后一个时间戳。
// 对应 testdata/alert_dlscdb1.log 的真实场景——10GB 样本末尾皆为 LOGMINER 续行。
func TestDetectLastTimeWithTrailingContinuations(t *testing.T) {
	p := writeTemp(t, strings.Join([]string{
		"Sat Mar 16 16:11:32 2019",
		"Starting ORACLE instance (normal)",
		"Sun Sep 20 09:15:26 2026",
		"LOGMINER: Read buffers: 8",
		"LOGMINER: Memory LWM: limit 10M, LWM 8M, 80%",
		"LOGMINER: Begin mining logfile for session -2147264255 thread 2",
	}, "\n")+"\n")
	rf, err := fileio.OpenReadOnly(p)
	if err != nil {
		t.Fatal(err)
	}
	defer rf.Close()

	rep, err := probe.Detect(rf)
	if err != nil {
		t.Fatal(err)
	}
	if rep.LastTime != "2026-09-20 09:15:26" {
		t.Errorf("末条时间 = %q，期望 2026-09-20 09:15:26", rep.LastTime)
	}
}

// 回归：尾部窗口整体无时间戳时，须向前扩展窗口回溯，而不是直接判为查不到。
func TestDetectLastTimeBeyondTailWindow(t *testing.T) {
	// 构造超过一个采样窗口（64KiB）的无时间戳续行，把真实时间戳挤到上一窗口。
	var lines []string
	lines = append(lines, "Sat Mar 16 16:11:32 2019", "Sun Sep 20 09:15:26 2026")
	for i := 0; i < 2000; i++ {
		lines = append(lines, fmt.Sprintf("LOGMINER: continuation line %d padding padding padding", i))
	}
	p := writeTemp(t, strings.Join(lines, "\n")+"\n")
	rf, err := fileio.OpenReadOnly(p)
	if err != nil {
		t.Fatal(err)
	}
	defer rf.Close()

	rep, err := probe.Detect(rf)
	if err != nil {
		t.Fatal(err)
	}
	if rep.LastTime != "2026-09-20 09:15:26" {
		t.Errorf("跨窗口回溯失败：末条时间 = %q，期望 2026-09-20 09:15:26", rep.LastTime)
	}
}
