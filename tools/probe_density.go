//go:build probe

// 采样密度探针：仅用于人工估算大文件时间分布密度的实验脚本，不参与常规构建。
// 用法：go run -tags probe ./tools/probe_density.go
package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"sort"
	"time"
)

var re = regexp.MustCompile(`^[A-Z][a-z]{2} [A-Z][a-z]{2} \d{2} \d{2}:\d{2}:\d{2} \d{4}`)

func main() {
	f, err := os.Open("testdata/alert_dlscdb1.log")
	if err != nil {
		panic(err)
	}
	st, _ := f.Stat()
	size := st.Size()
	const step = 4 << 20
	const win = 1 << 10
	var times []int64
	var blocks, hasTS int
	step64 := int64(step)
	win64 := int64(win)
	for off := int64(0); off < size; off += step64 {
		blocks++
		buf := make([]byte, win64)
		n, _ := f.ReadAt(buf, off)
		ln := bufio.NewScanner(newReaderAt(f, off, int64(n)))
		for ln.Scan() {
			line := ln.Bytes()
			m := re.Find(line)
			if m == nil {
				continue
			}
			if t, err := time.Parse("Mon Jan 02 15:04:05 2006", string(m)); err == nil {
				times = append(times, t.Unix())
				hasTS++
			}
		}
	}
	fmt.Printf("采样块数=%d 含时间戳块=%d (%.1f%%)\n", blocks, hasTS, float64(hasTS)*100/float64(blocks))
	if len(times) < 2 {
		return
	}
	fmt.Printf("时间戳采样点数=%d\n", len(times))
	first := time.Unix(times[0], 0).Format("2006-01-02 15:04:05")
	last := time.Unix(times[len(times)-1], 0).Format("2006-01-02 15:04:05")
	fmt.Printf("首=%s 末=%s 跨度=%s\n", first, last, time.Since(time.Unix(times[0], 0)).Round(time.Hour))
	// 单调性检查
	rev := 0
	maxRev := int64(0)
	for i := 1; i < len(times); i++ {
		if times[i] < times[i-1] {
			rev++
			if d := times[i-1] - times[i]; d > maxRev {
				maxRev = d
			}
		}
	}
	fmt.Printf("递减次数=%d 最大回退=%ds\n", rev, maxRev)
	// 时间戳间距离（内容行数量）分布
	diffs := make([]int64, 0, len(times)-1)
	for i := 1; i < len(times); i++ {
		diffs = append(diffs, times[i]-times[i-1])
	}
	sort.Slice(diffs, func(i, j int) bool { return diffs[i] < diffs[j] })
	p := func(q float64) int64 { return diffs[int(float64(len(diffs))*q)] }
	fmt.Printf("时间戳间隔秒数: P50=%d P90=%d P99=%d 最大=%d\n", p(0.5), p(0.9), p(0.99), diffs[len(diffs)-1])
}

type readerAtSlice struct {
	f      *os.File
	off, n int64
}

func newReaderAt(f *os.File, off, n int64) *readerAtSlice {
	return &readerAtSlice{f: f, off: off, n: n}
}
func (r *readerAtSlice) Read(p []byte) (int, error) {
	if r.n <= 0 {
		return 0, os.ErrDeadlineExceeded
	}
	if len(p) > int(r.n) {
		p = p[:r.n]
	}
	k, err := r.f.ReadAt(p, r.off)
	r.off += int64(k)
	r.n -= int64(k)
	if r.n <= 0 {
		return k, err
	}
	return k, err
}
