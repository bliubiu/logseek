package timefmt_test

import (
	"testing"
	"time"

	"github.com/bliubiu/logseek/internal/domain/timefmt"
)

func TestDetectCommon(t *testing.T) {
	lay, err := timefmt.Detect([]string{"2026-01-01 08:00:00.123 hello"})
	if err != nil {
		t.Fatal(err)
	}
	if lay.Layout == "" {
		t.Fatal("布局为空")
	}
}

func TestDetectFail(t *testing.T) {
	_, err := timefmt.Detect([]string{"no time here", "still nothing"})
	if err == nil {
		t.Fatal("应失败")
	}
}

func TestParseLine(t *testing.T) {
	lay := timefmt.Layout{Layout: "2006-01-02 15:04:05"}
	ts, err := timefmt.ParseLine(lay, "2026-03-04 05:06:07 msg")
	if err != nil {
		t.Fatal(err)
	}
	if ts.Day() != 4 {
		t.Errorf("解析日 = %d", ts.Day())
	}
}

// 回归：Oracle alert 日志格式必须识别为带年份的完整布局，
// 不得退化为丢失年份的 "Jan 02 15:04:05"。
func TestDetectOracleAlert(t *testing.T) {
	samples := []string{
		"Sat Mar 16 16:11:32 2019",
		"Sun Sep 20 09:15:00 2026",
	}
	lay, err := timefmt.Detect(samples)
	if err != nil {
		t.Fatal(err)
	}
	if lay.Layout != "Mon Jan 02 15:04:05 2006" {
		t.Fatalf("布局 = %q，期望 Mon Jan 02 15:04:05 2006", lay.Layout)
	}
}

func TestParseOracleAlert(t *testing.T) {
	lay := timefmt.Layout{Layout: "Mon Jan 02 15:04:05 2006"}
	ts, ok := timefmt.ParseWith(lay.Layout, "Sat Mar 16 16:11:32 2019")
	if !ok {
		t.Fatal("解析失败")
	}
	if ts.Year() != 2019 || ts.Month() != time.March || ts.Day() != 16 {
		t.Errorf("解析结果 = %v", ts)
	}
}

// 无数字行不可能含时间戳，应快速判定失败（性能护栏）。
func TestParseWithNoDigitFastFail(t *testing.T) {
	lay := timefmt.Layout{Layout: "Mon Jan 02 15:04:05 2006"}
	for _, line := range []string{
		"Adjusting the default value of parameter parallel_max_servers",
		"Starting ORACLE instance (normal)",
		"", "\t  ",
	} {
		if _, ok := timefmt.ParseWith(lay.Layout, line); ok {
			t.Errorf("行 %q 不应解析出时间", line)
		}
	}
}

// 性能护栏：坏行（无时间戳）解析必须远快于任意字符串做 time.Parse 全路径穷举。
func BenchmarkParseWithBadLine(b *testing.B) {
	lay := "Mon Jan 02 15:04:05 2006"
	lines := []string{
		"Adjusting the default value of parameter parallel_max_servers",
		"LOGMINER: Begin mining logfile for session -2147264255 thread 1 sequence 134013",
		"Large Pages unused system wide = 0 (0 KB)",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, l := range lines {
			timefmt.ParseWith(lay, l)
		}
	}
}

func BenchmarkParseWithOracleAlert(b *testing.B) {
	lay := "Mon Jan 02 15:04:05 2006"
	line := "Sat Mar 16 16:11:32 2019"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		timefmt.ParseWith(lay, line)
	}
}
