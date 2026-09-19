// Package application 编排预检、切片、检索、导出用例。
package application

import (
	"fmt"
	"time"

	"github.com/bliubiu/logseek/internal/domain/probe"
	"github.com/bliubiu/logseek/internal/domain/search"
	"github.com/bliubiu/logseek/internal/domain/stream"
	"github.com/bliubiu/logseek/internal/domain/timefmt"
	"github.com/bliubiu/logseek/internal/domain/timeslice"
	"github.com/bliubiu/logseek/internal/infrastructure/fileio"
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
	EnableTime   bool
	StartTime    time.Time
	EndTime      time.Time
	TimeFormat   string // 显式格式；空则探测

	// 内容条件
	EnableSearch bool
	Keywords     []string
	And          bool
	IgnoreCase   bool
	WholeWord    bool
	Pattern      string
}

// Export 执行导出并返回摘要。
func Export(req ExportRequest) (stream.Summary, error) {
	if req.Source == "" {
		return stream.Summary{}, fmt.Errorf("必须指定源日志路径")
	}
	var filters []stream.LineFilter
	var badCounter *int64

	if req.EnableTime {
		layout, err := resolveLayout(req)
		if err != nil {
			return stream.Summary{}, err
		}
		flt, err := timeslice.New(layout, req.StartTime, req.EndTime, timeslice.DefaultPolicy())
		if err != nil {
			return stream.Summary{}, err
		}
		// 包装以累计坏行到摘要
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

	sum, err := stream.Run(req.Source, sk, filters, stream.DefaultOptions())
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
