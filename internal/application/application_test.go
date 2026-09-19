package application_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bliubiu/logseek/internal/application"
	"github.com/bliubiu/logseek/internal/domain/stream"
	"github.com/bliubiu/logseek/internal/infrastructure/resources"
)

func TestResourcesDefaultLimit(t *testing.T) {
	application.ApplyResources()
	lim := resources.CurrentMemoryLimit()
	if lim != resources.DefaultMemoryLimitBytes {
		t.Fatalf("软内存上限 = %d, 期望 %d", lim, resources.DefaultMemoryLimitBytes)
	}
}

func TestExportTimeAndKeyword(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "app.log")
	body := strings.Join([]string{
		"2026-01-01 10:00:00.000 [ERROR] 数据库连接失败",
		"2026-01-01 10:00:01.000 [INFO] 心跳正常",
		"2026-01-01 10:30:00.000 [ERROR] 再次失败",
		"2026-01-01 12:00:00.000 [ERROR] 超窗外",
	}, "\n") + "\n"
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.log")

	st, _ := time.ParseInLocation("2006-01-02 15:04:05", "2026-01-01 10:00:00", time.Local)
	en, _ := time.ParseInLocation("2006-01-02 15:04:05", "2026-01-01 11:00:00", time.Local)

	sum, err := application.Export(application.ExportRequest{
		Source:       src,
		Output:       out,
		EnableTime:   true,
		EnableSearch: true,
		StartTime:    st,
		EndTime:      en,
		TimeFormat:   "2006-01-02 15:04:05.000",
		Keywords:     []string{"ERROR"},
		IgnoreCase:   true,
		EnableMask:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Matched != 2 {
		t.Errorf("命中 = %d, 期望 2（窗口内 ERROR）", sum.Matched)
	}
	got, _ := os.ReadFile(out)
	text := string(got)
	if strings.Contains(text, "超窗外") || strings.Contains(text, "心跳正常") {
		t.Errorf("输出混入窗口外或非关键词行:\n%s", text)
	}
	if !strings.Contains(text, "数据库连接失败") || !strings.Contains(text, "再次失败") {
		t.Errorf("缺少期望命中行:\n%s", text)
	}
}

func TestExportRelative(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "app.log")
	now := time.Now()
	inside := now.Add(-10 * time.Minute).Format("2006-01-02 15:04:05")
	outside := now.Add(-48 * time.Hour).Format("2006-01-02 15:04:05")
	body := inside + " [ERROR] 近期\n" + outside + " [ERROR] 过旧\n"
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	sum, err := application.Export(application.ExportRequest{
		Source:     src,
		EnableTime: true,
		Relative:   "30m",
		Now:        now,
		TimeFormat: "2006-01-02 15:04:05",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Matched != 1 {
		t.Fatalf("相对窗口命中=%d 期望1", sum.Matched)
	}
}

func TestInspectApplication(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.log")
	_ = os.WriteFile(p, []byte("2026-01-01 08:00:00.123 ok\n"), 0o644)
	rep, err := application.Inspect(p)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.TimeDetected {
		t.Error("应检测时间")
	}
}

func TestFormatSummaryJSON(t *testing.T) {
	sum := stream.Summary{Scanned: 3, Matched: 1}
	s := application.FormatSummary(sum, true, true)
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("非 JSON: %s", s)
	}
	if m["scanned"].(float64) != 3 {
		t.Fatal("scanned 错误")
	}
	// 中文摘要
	s2 := application.FormatSummary(sum, false, true)
	if !strings.Contains(s2, "扫描行") {
		t.Fatal("中文摘要缺失")
	}
}

func TestExportMissingSource(t *testing.T) {
	_, err := application.Export(application.ExportRequest{})
	if err == nil || !strings.Contains(err.Error(), "源日志") {
		t.Fatal("应报中文错误")
	}
}
