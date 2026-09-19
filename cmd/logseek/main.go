// Package main logSeek 命令行入口。
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/bliubiu/logseek/internal/application"
)

// 退出码定义（可被脚本消费）。
const (
	exitOK          = 0
	exitRuntime     = 1
	exitUsage       = 2
	exitNotFound    = 3
	exitProbeFailed = 4
)

var (
	version = "0.1.0"
	// 资源相关
	memLimitMiB int
	// 时间
	timeStart string
	timeEnd   string
	timeFmt   string
	// 检索
	keywords   []string
	andMatch   bool
	ignoreCase bool
	wholeWord  bool
	pattern    string
	output     string
)

func main() {
	application.ApplyResources()

	root := &cobra.Command{
		Use:   "logseek",
		Short: "大文件日志解析工具（只读源日志）",
		Long:  "logSeek：面向运维的大文本日志命令行解析工具。全程只读源日志，只写输出结果文件。",
		Version: version,
	}

	root.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		if memLimitMiB > 0 {
			application.ApplyResources()
		}
	}

	root.PersistentFlags().IntVar(&memLimitMiB, "mem-limit-mib", 192, "Go 软内存上限（MiB），不得为 0 关闭")

	root.AddCommand(inspectCmd(), sliceCmd(), grepCmd(), exportCmd())

	if err := root.Execute(); err != nil {
		// cobra 已打印
		os.Exit(exitUsage)
	}
}

func inspectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "inspect <文件路径>",
		Short: "头尾随机预检：编码、时间格式、首末时间",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rep, err := application.Inspect(args[0])
			if err != nil {
				fmt.Fprintln(os.Stderr, "错误:", err)
				os.Exit(mapExit(err, false))
			}
			fmt.Print(rep.Format())
			return nil
		},
	}
}

func sliceCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "slice <文件路径>",
		Short: "按时间窗口切片导出",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			start, end, err := parseWindow()
			if err != nil {
				fmt.Fprintln(os.Stderr, "错误:", err)
				os.Exit(exitUsage)
			}
			req := application.ExportRequest{
				Source:     args[0],
				Output:     output,
				EnableTime: true,
				StartTime:  start,
				EndTime:    end,
				TimeFormat: timeFmt,
			}
			sum, err := application.Export(req)
			if err != nil {
				fmt.Fprintln(os.Stderr, "错误:", err)
				os.Exit(mapExit(err, true))
			}
			fmt.Println(sum.Format())
			if output != "" {
				fmt.Println("输出文件:", output)
			}
			return nil
		},
	}
	bindTimeFlags(c)
	c.Flags().StringVarP(&output, "output", "o", "", "输出结果文件路径（可选）")
	return c
}

func grepCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "grep <文件路径>",
		Short: "内容检索导出（关键词/正则）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(keywords) == 0 && pattern == "" {
				fmt.Fprintln(os.Stderr, "错误: 必须指定至少一个关键词或 --pattern")
				os.Exit(exitUsage)
			}
			req := application.ExportRequest{
				Source:       args[0],
				Output:       output,
				EnableSearch: true,
				Keywords:     keywords,
				And:          andMatch,
				IgnoreCase:   ignoreCase,
				WholeWord:    wholeWord,
				Pattern:      pattern,
			}
			sum, err := application.Export(req)
			if err != nil {
				fmt.Fprintln(os.Stderr, "错误:", err)
				os.Exit(mapExit(err, true))
			}
			fmt.Println(sum.Format())
			if output != "" {
				fmt.Println("输出文件:", output)
			}
			return nil
		},
	}
	bindSearchFlags(c)
	c.Flags().StringVarP(&output, "output", "o", "", "输出结果文件路径（可选）")
	return c
}

func exportCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "export <文件路径>",
		Short: "时间 ∧ 内容组合导出",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			hasTime := timeStart != "" || timeEnd != ""
			hasSearch := len(keywords) > 0 || pattern != ""
			if !hasTime && !hasSearch {
				fmt.Fprintln(os.Stderr, "错误: export 至少需要时间窗口或检索条件")
				os.Exit(exitUsage)
			}
			req := application.ExportRequest{
				Source:       args[0],
				Output:       output,
				EnableTime:   hasTime,
				EnableSearch: hasSearch,
				Keywords:     keywords,
				And:          andMatch,
				IgnoreCase:   ignoreCase,
				WholeWord:    wholeWord,
				Pattern:      pattern,
				TimeFormat:   timeFmt,
			}
			if hasTime {
				start, end, err := parseWindow()
				if err != nil {
					fmt.Fprintln(os.Stderr, "错误:", err)
					os.Exit(exitUsage)
				}
				req.StartTime = start
				req.EndTime = end
			}
			sum, err := application.Export(req)
			if err != nil {
				fmt.Fprintln(os.Stderr, "错误:", err)
				os.Exit(mapExit(err, true))
			}
			fmt.Println(sum.Format())
			if output != "" {
				fmt.Println("输出文件:", output)
			}
			return nil
		},
	}
	bindTimeFlags(c)
	bindSearchFlags(c)
	c.Flags().StringVarP(&output, "output", "o", "", "输出结果文件路径")
	return c
}

func bindTimeFlags(c *cobra.Command) {
	c.Flags().StringVar(&timeStart, "start", "", "起始时间，如 2026-01-01 00:00:00")
	c.Flags().StringVar(&timeEnd, "end", "", "结束时间（不含），如 2026-01-02 00:00:00")
	c.Flags().StringVar(&timeFmt, "time-format", "", "显式时间格式（Go layout），探测失败时使用")
}

func bindSearchFlags(c *cobra.Command) {
	c.Flags().StringArrayVarP(&keywords, "keyword", "k", nil, "关键词（可多次）")
	c.Flags().BoolVar(&andMatch, "and", false, "多关键词按与关系（默认为或）")
	c.Flags().BoolVar(&ignoreCase, "ignore-case", true, "忽略大小写")
	c.Flags().BoolVar(&wholeWord, "word-regexp", false, "整词匹配")
	c.Flags().StringVar(&pattern, "pattern", "", "RE2 正则模式（复杂模式兜底）")
}

func parseWindow() (time.Time, time.Time, error) {
	if timeStart == "" || timeEnd == "" {
		return time.Time{}, time.Time{}, fmt.Errorf("必须同时指定 --start 与 --end")
	}
	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05.000",
		time.RFC3339,
		"2006-01-02",
	}
	parse := func(s string) (time.Time, error) {
		for _, l := range layouts {
			if t, err := time.ParseInLocation(l, strings.TrimSpace(s), time.Local); err == nil {
				return t, nil
			}
		}
		return time.Time{}, fmt.Errorf("无法解析时间 %q，请使用 2006-01-02 15:04:05", s)
	}
	st, err := parse(timeStart)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	en, err := parse(timeEnd)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return st, en, nil
}

func mapExit(err error, isExport bool) int {
	if err == nil {
		return exitOK
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "文件不存在"), strings.Contains(msg, "没有读取权限"):
		return exitNotFound
	case strings.Contains(msg, "未能识别时间格式"), strings.Contains(msg, "无法按格式"), strings.Contains(msg, "文件为空"):
		return exitProbeFailed
	default:
		if isExport {
			return exitRuntime
		}
		return exitProbeFailed
	}
}
