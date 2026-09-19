package timefmt_test

import (
	"testing"

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
