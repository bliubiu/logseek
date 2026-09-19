// Package application 编排预检、切片、检索、导出用例。
package application

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/bliubiu/logseek/internal/domain/probe"
	"github.com/bliubiu/logseek/internal/domain/search"
	"github.com/bliubiu/logseek/internal/domain/stream"
	"github.com/bliubiu/logseek/internal/domain/timefmt"
	"github.com/bliubiu/logseek/internal/domain/timeslice"
	"github.com/bliubiu/logseek/internal/infrastructure/fileio"
	"github.com/bliubiu/logseek/internal/infrastructure/logging"
	"github.com/bliubiu/logseek/internal/infrastructure/mask"
	"github.com/bliubiu/logseek/internal/infrastructure/resources"
	"github.com/bliubiu/logseek/internal/infrastructure/sink"
)

// Inspect 预检用例。
func Inspect(path string) (probe.Report, error) {
	rf, err := fileio.OpenReadOnly(path)
	if err != nil {
		return probe.Report{}, err
	}
	defer rf.Close()
	return probe.Detect(rf)
}

// ExportRequest 组合导出请求。
type ExportRequest struct {
	Source string
	Output string // 为空则不落盘（摘要模式）

	// 时间窗口
	EnableTime bool
	StartTime  time.Time
	EndTime    time.Time
	TimeFormat string // 显式格式；空则探测
	Relative   string // 相对时间如 30m/7d/2h（与绝对互斥，优先）
	Now        time.Time // 测试注入

	// 内容条件
	EnableSearch bool
	Keywords     []string
	And          bool
	IgnoreCase   bool
	WholeWord    bool
	Pattern      string

	// 资源
	ReadRate   int64
	EnableMask bool

	// 日志
	Logger *logging.Logger
}

// Export 执行导出并返回摘要。
func Export(req ExportRequest) (stream.Summary, error) {
	if req.Source == "" {
		return stream.Summary{}, fmt.Errorf("必须指定源日志路径")
	}
	var filters []stream.LineFilter
	var badCounter *int64

	if req.EnableTime {
		start, end := req.StartTime, req.EndTime
		if req.Relative != "" {
			now := req.Now
			if now.IsZero() {
				now = time.Now()
			}
			s, e, err := timeslice.RelativeWindow(req.Relative, now)
			if err != nil {
				return stream.Summary{}, err
			}
			start, end = s, e
		}
		layout, err := resolveLayout(req)
		if err != nil {
			return stream.Summary{}, err
		}
		flt, err := timeslice.New(layout, start, end, timeslice.DefaultPolicy())
		if err != nil {
			return stream.Summary{}, err
		}
		filters = append(filters, stream.FilterFunc(func(line []byte) bool {
			return flt.Accept(line)
		}))
		bad := &flt.BadLines
		badCounter = bad
	}

	if req.EnableSearch {
		combine := search.CombineOr
		if req.And {
			combine = search.CombineAnd
		}
		cond := search.Condition{
			Keywords:      req.Keywords,
			Combine:       combine,
			CaseSensitive: !req.IgnoreCase,
			WholeWord:     req.WholeWord,
			Pattern:       req.Pattern,
		}
		m, err := search.Build(cond)
		if err != nil {
			return stream.Summary{}, err
		}
		filters = append(filters, stream.FilterFunc(func(line []byte) bool {
			return m.Match(line)
		}))
	}

	var sk sink.SinkCloser
	var fs *sink.FileSink
	if req.Output != "" {
		fs, err := sink.New(req.Output)
		if err != nil {
			return stream.Summary{}, err
		}
		sk = fs
	} else {
		fs = sink.NewDiscard()
		sk = fs
	}

	opt := stream.DefaultOptions()
	opt.ReadRate = req.ReadRate

	sum, err := stream.Run(req.Source, sk, filters, opt)
	if badCounter != nil {
		sum.BadLines = *badCounter
	}
	if err != nil {
		return sum, err
	}
	return sum, nil
}

func resolveLayout(req ExportRequest) (timefmt.Layout, error) {
	if req.TimeFormat != "" {
		return timefmt.Layout{Layout: req.TimeFormat, FromUser: true}, nil
	}
	rep, err := Inspect(req.Source)
	if err != nil {
		return timefmt.Layout{}, err
	}
	if !rep.TimeDetected {
		return timefmt.Layout{}, fmt.Errorf("%s", rep.Hint)
	}
	return timefmt.Layout{Layout: rep.TimeLayout}, nil
}

// ApplyResources 启动资源基线。
func ApplyResources() {
	resources.ApplyMemoryLimit(0)
}

// FormatSummary 输出摘要；jsonMode 为 true 时输出 JSON。
func FormatSummary(sum stream.Summary, jsonMode bool, enableMask bool) string {
	if jsonMode {
		b, err := json.Marshal(sum.JSON())
		if err != nil {
			return sum.Format()
		}
		return string(b)
	}
	s := sum.Format()
	if enableMask {
		s = mask.Apply(s)
	}
	return s
}
