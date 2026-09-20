package timeslice_test

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/bliubiu/logseek/internal/domain/timefmt"
	"github.com/bliubiu/logseek/internal/domain/timeslice"
)

const locateLayout = "2006-01-02 15:04:05"

// memAt 内存随机读，模拟 ReaderAt。
type memAt struct{ b []byte }

func (m *memAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(m.b)) {
		return 0, io.EOF
	}
	n := copy(p, m.b[off:])
	return n, nil
}
func (m *memAt) Size() (int64, error) { return int64(len(m.b)), nil }

func locateWindow(t *testing.T, b []byte, s, e string, opts timeslice.LocateOptions) timeslice.Span {
	t.Helper()
	st, _ := time.ParseInLocation(locateLayout, s, time.Local)
	en, _ := time.ParseInLocation(locateLayout, e, time.Local)
	lay := timefmt.Layout{Layout: locateLayout}
	sp, err := timeslice.Locate(&memAt{b: b}, lay, timeslice.Window{Start: st, End: en}, opts)
	if err != nil {
		t.Fatal(err)
	}
	return sp
}

// buildBlocks 构造分块日志：每块第一行为递增时间戳行，其余为无时间戳填充。
// blockSize 为每块字节数；blocks 为块数；stepSec 为相邻块时间戳递增秒数；
// gapAt>=0 时该块全部为无时间戳（模拟大段无锚点区域）。
// reverseAt>=0 时该块时间戳回跳（模拟乱序日志）。
func buildBlocks(t *testing.T, blockSize int, blocks int, stepSec int, gapAt, reverseAt int) []byte {
	t.Helper()
	var sb strings.Builder
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.Local)
	for i := 0; i < blocks; i++ {
		var head byte
		if i == reverseAt && reverseAt > 0 {
			head = 0
		} else {
			head = 1
		}
		if i == gapAt {
			head = 0 // 巨区：无时间戳
		}
		_ = head
		ts := base.Add(time.Duration(i*stepSec) * time.Second)
		if i == reverseAt && reverseAt > 0 {
			ts = base.Add(time.Duration((i-2)*stepSec) * time.Second) // 回跳
		}
		line := ts.Format(locateLayout) + " seqline\n"
		line += strings.Repeat("x", 60) + fmt.Sprintf(" payload-%d\n", i)
		if i == gapAt {
			line = strings.Repeat("y", 60) + fmt.Sprintf(" nopayload-%d\n", i)
		}
		for len(line) < blockSize {
			line += "filler\n"
		}
		sb.WriteString(line[:blockSize])
	}
	return []byte(sb.String())
}

func locateOpts(blockSize int) timeslice.LocateOptions {
	o := timeslice.DefaultLocateOptions()
	o.Step = int64(blockSize)
	o.Probe = int64(blockSize)
	return o
}

func TestLocateSpanSeekBasic(t *testing.T) {
	const bsz = 4096
	b := buildBlocks(t, bsz, 100, 60, -1, -1) // 每块时间 +1min
	sp := locateWindow(t, b, "2026-01-01 10:30:00", "2026-01-01 10:35:00", locateOpts(bsz))
	if sp.Mode != timeslice.SpanSeek {
		t.Fatalf("模式 = %s, 期望 span-seek", sp.Mode)
	}
	// 保守块粒度：起点取「首个已知时间 ≥ start」块的前一块起点（块30→回退到块29）。
	wantStart := int64(29 * bsz)
	if sp.StartOff != wantStart {
		t.Errorf("StartOff = %d, 期望 %d", sp.StartOff, wantStart)
	}
	// 首个 ≥end(10:35) 时间戳块为块35 → 该块整体纳入，区间尾为其末尾。
	wantEnd := int64(35*bsz + bsz)
	if sp.EndOff != wantEnd {
		t.Errorf("EndOff = %d, 期望 %d", sp.EndOff, wantEnd)
	}
}

func TestLocateWindowBeforeFile(t *testing.T) {
	const bsz = 4096
	b := buildBlocks(t, bsz, 20, 60, -1, -1)
	sp := locateWindow(t, b, "2020-01-01 00:00:00", "2020-01-02 00:00:00", locateOpts(bsz))
	if sp.Mode != timeslice.SpanSeek {
		t.Fatalf("模式 = %s", sp.Mode)
	}
	if sp.StartOff != 0 || sp.EndOff != int64(len(b)) {
		t.Errorf("窗口早于文件应从文件头到文件尾，实际 [%d,%d), size=%d", sp.StartOff, sp.EndOff, len(b))
	}
	// 窗口在文件所有时间戳之后则应空
	sp2 := locateWindow(t, b, "2027-01-01 00:00:00", "2027-01-02 00:00:00", locateOpts(bsz))
	if !sp2.IsEmpty() || sp2.StartOff != int64(len(b)) {
		t.Errorf("窗口晚于文件应空区间，实际 [%d,%d)", sp2.StartOff, sp2.EndOff)
	}
}

func TestLocateWindowAfterFileEnd(t *testing.T) {
	const bsz = 4096
	b := buildBlocks(t, bsz, 20, 60, -1, -1)
	sp := locateWindow(t, b, "2026-01-01 08:00:00", "2027-01-01 00:00:00", locateOpts(bsz))
	if sp.StartOff != 0 {
		t.Errorf("StartOff = %d, 期望 0", sp.StartOff)
	}
	if sp.EndOff != int64(len(b)) {
		t.Errorf("EndOff = %d, 期望 file end %d", sp.EndOff, len(b))
	}
}

func TestLocateFallbackNonMonotonic(t *testing.T) {
	const bsz = 4096
	b := buildBlocks(t, bsz, 20, 60, -1, 10)
	sp := locateWindow(t, b, "2026-01-01 10:00:00", "2026-01-01 10:20:00", locateOpts(bsz))
	if sp.Mode != timeslice.FullScan {
		t.Fatalf("乱序应回退 full-scan，实际 %s", sp.Mode)
	}
	if sp.Reason == "" {
		t.Errorf("回退必须给出原因说明")
	}
}

func TestLocateFallbackNoTimestamps(t *testing.T) {
	b := []byte(strings.Repeat("nothing here\n", 2000))
	sp := locateWindow(t, b, "2026-01-01 10:00:00", "2026-01-01 11:00:00", locateOpts(4096))
	if sp.Mode != timeslice.FullScan {
		t.Fatalf("无时间戳应回退 full-scan，实际 %s", sp.Mode)
	}
	if sp.Reason == "" {
		t.Errorf("回退必须给出原因说明")
	}
}

func TestLocateFallbackTooSmall(t *testing.T) {
	const bsz = 4096
	b := buildBlocks(t, bsz, 50, 60, -1, -1) // 文件只有 1 块（step 4KiB→ 50 块远超，调整）
	_ = b
	bb := buildBlocks(t, bsz, 1, 60, -1, -1)
	sp := locateWindow(t, bb, "2026-01-01 10:00:00", "2026-01-01 10:10:00", locateOpts(bsz))
	if sp.Mode != timeslice.FullScan {
		t.Fatalf("单块文件应回退 full-scan，实际 %s", sp.Mode)
	}
	if sp.Reason == "" {
		t.Errorf("回退必须给出原因说明")
	}
}

func TestLocateGapBlockCarryForward(t *testing.T) {
	const bsz = 4096
	// 块10 巨区（无时间戳），其后块11 时间 +1min（2026-01-01 10:11）
	b := buildBlocks(t, bsz, 20, 60, 10, -1)
	_ = b
	// 窗口跨入巨区内部：10:10~10:12，起点应为块10起点（其时间沿用 10:09）
	sp := locateWindow(t, b, "2026-01-01 10:10:00", "2026-01-01 10:12:00", locateOpts(bsz))
	if sp.Mode != timeslice.SpanSeek {
		t.Fatalf("模式 = %s", sp.Mode)
	}
	if sp.StartOff != int64(10*bsz) {
		t.Errorf("StartOff = %d, 期望块10起点 %d", sp.StartOff, 10*bsz)
	}
}

func TestLocateOptionsDefaults(t *testing.T) {
	o := timeslice.DefaultLocateOptions()
	if o.Step <= 0 || o.Probe <= 0 || o.Max <= 0 {
		t.Fatalf("默认参数非法: %+v", o)
	}
}

var _ = bytes.Compare
