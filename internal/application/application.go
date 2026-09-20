// Package application 编排预检、切片、检索、导出用例。
package application

import (
	"encoding/json"
	"time"

	"github.com/bliubiu/logseek/internal/domain/errkind"
	"github.com/bliubiu/logseek/internal/domain/probe"
	"github.com/bliubiu/logseek/internal/domain/search"
	"github.com/bliubiu/logseek/internal/domain/stream"
	"github.com/bliubiu/logseek/internal/domain/timefmt"
	"github.com/bliubiu/logseek/internal/domain/timeslice"
	"github.com/bliubiu/logseek/internal/infrastructure/fileio"
	"github.com/bliubiu/logseek/internal/infrastructure/logging"
	"github.com/bliubiu/logseek/internal/infrastructure/mask"
	"github.com/bliubiu/logseek/internal/infrastructure/ratelimit"
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
	TimeFormat string    // 显式格式；空则探测
	Relative   string    // 相对时间如 30m/7d/2h（与绝对互斥，优先）
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

	// DisableStickyTime 关闭时间戳继承：续行不再沿用上一条时间戳。
	// 默认开启继承以覆盖 Oracle alert 等多行日志；显式关闭后退化为逐行独立判定。
	DisableStickyTime bool

	// 日志
	Logger *logging.Logger
}

// Export 执行导出并返回摘要。
func Export(req ExportRequest) (stream.Summary, error) {
	if req.Source == "" {
		return stream.Summary{}, errkind.New(errkind.KindUsage, "必须指定源日志路径")
	}
	var filters []stream.LineFilter
	var badCounter *int64

	// 时间窗口、布局与稀疏定位结果；供扫描区间裁剪使用。
	var start, end time.Time
	var layout timefmt.Layout
	var span timeslice.Span

	if req.EnableTime {
		start, end = req.StartTime, req.EndTime
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
		var err error
		layout, err = resolveLayout(req)
		if err != nil {
			return stream.Summary{}, err
		}
		flt, err := timeslice.New(layout, start, end, timeslicePolicy(req))
		if err != nil {
			return stream.Summary{}, err
		}
		filters = append(filters, stream.FilterFunc(func(line []byte) bool {
			return flt.Accept(line)
		}))
		bad := &flt.BadLines
		badCounter = bad

		// 稀疏定位：把全文件扫描裁剪到窗口所在字节区间。
		// 定位不可靠时 Locate 返回 FullScan，调用方整文件扫描，语义不变。
		sp, lerr := locateSpan(req.Source, layout, start, end)
		if lerr != nil {
			return stream.Summary{}, lerr
		}
		span = sp
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
	if req.Output != "" {
		var so sink.Options
		// 导出文件与控制台摘要同口径脱敏；关闭 --mask 时原样写出。
		if req.EnableMask {
			so.Mask = mask.Apply
		}
		fs, err := sink.NewWithOptions(req.Output, so)
		if err != nil {
			return stream.Summary{}, err
		}
		sk = fs
	} else {
		sk = sink.NewDiscard()
	}

	// 端口装配：domain 不感知基础设施实现，此处统一注入。
	opt := stream.DefaultOptions()
	op := fileio.NewOpener()
	opt.Opener = op
	opt.RandomOpener = op
	if req.ReadRate > 0 {
		opt.Limiter = ratelimit.New(req.ReadRate)
	}
	// 定位成功时按区间裁剪读取量；FullScan 不设区间，保持全文件语义。
	if span.Mode == timeslice.SpanSeek {
		opt.SpanStart, opt.SpanEnd = span.StartOff, span.EndOff
	}

	sum, err := stream.Run(req.Source, sk, filters, opt)
	if req.EnableTime {
		sum.LocateMode = string(span.Mode)
	}
	if badCounter != nil {
		sum.BadLines = *badCounter
	}
	if err != nil {
		return sum, err
	}
	return sum, nil
}

// timeslicePolicy 过滤器策略。
//
// 默认启用时间戳继承：无时间戳的续行沿用上一条时间戳，
// 保证多行日志（Oracle alert 等）的报错正文与其时间戳一起被切片。
func timeslicePolicy(req ExportRequest) timeslice.Policy {
	p := timeslice.DefaultPolicy()
	if req.DisableStickyTime {
		p.StickyTime = false
	}
	return p
}

// locateSpan 按时间窗口做稀疏采样二分定位，返回应扫描的字节区间。
//
// 输入文件保持只读；定位不可靠（时间戳过少、乱序、文件过小）时返回 Mode=FullScan，
// 由调用方整文件扫描，结果语义与不计区间完全一致。
func locateSpan(src string, layout timefmt.Layout, start, end time.Time) (timeslice.Span, error) {
	rf, err := fileio.OpenReadOnly(src)
	if err != nil {
		return timeslice.Span{}, err
	}
	defer rf.Close()
	return timeslice.Locate(rf, layout, timeslice.Window{Start: start, End: end}, timeslice.DefaultLocateOptions())
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
		return timefmt.Layout{}, errkind.New(errkind.KindProbeFailed, "%s", rep.Hint)
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
