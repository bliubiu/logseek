// Package main logSeek 命令行入口。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/bliubiu/logseek/internal/application"
	"github.com/bliubiu/logseek/internal/domain/errkind"
	"github.com/bliubiu/logseek/internal/infrastructure/config"
	"github.com/bliubiu/logseek/internal/infrastructure/crypto"
	"github.com/bliubiu/logseek/internal/infrastructure/logging"
)

// 退出码定义（可被脚本消费）。
const (
	exitOK          = 0
	exitRuntime     = 1
	exitUsage       = 2
	exitNotFound    = 3
	exitProbeFailed = 4
	exitInterrupt   = 130
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
	// 全局
	configPath   string
	saveConfig   string
	ensurePwd    bool
	jsonOut      bool
	readRate     int64
	logLevel     string
	logDir       string
	enableMask   bool
	noStickyTime bool
	keyFilePath  string
	// 运行时
	appLogger *logging.Logger
	appConfig *config.Config
)

func main() {
	application.ApplyResources()

	root := &cobra.Command{
		Use:     "logseek",
		Short:   "大文件日志解析工具（只读源日志）",
		Long:    "logSeek：面向运维的大文本日志命令行解析工具。全程只读源日志，只写输出结果文件。",
		Version: version,
	}

	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		return initRuntime()
	}
	root.PersistentPostRun = func(cmd *cobra.Command, args []string) {
		// 命令成功后再落盘，失败路径不写配置（避免覆盖原文件）
		if err := maybeSaveConfig(); err != nil {
			fmt.Fprintln(os.Stderr, "错误:", err)
			return
		}
		if appLogger != nil {
			appLogger.Close()
		}
	}

	pf := root.PersistentFlags()
	pf.IntVar(&memLimitMiB, "mem-limit-mib", 192, "Go 软内存上限（MiB），不得为 0 关闭")
	pf.StringVar(&configPath, "config", "", "配置文件路径（JSON）")
	pf.StringVar(&saveConfig, "save-config", "", "把当前生效配置落盘到该路径（口令以 ENC 加密保存）")
	pf.BoolVar(&ensurePwd, "ensure-password", false, "配合 --save-config：口令为空时自动生成强口令")
	pf.StringVar(&keyFilePath, "key-file", "", "密钥文件路径（默认 .logseek/key）")
	pf.BoolVar(&jsonOut, "json", false, "以 JSON 输出摘要/报告")
	pf.Int64Var(&readRate, "read-rate", 0, "读限速（字节/秒），0 不限；生产建议 67108864（64MiB/s）")
	pf.StringVar(&logLevel, "log-level", "", "日志级别 DEBUG/INFO/ERROR")
	pf.StringVar(&logDir, "log-dir", "", "日志目录（默认 logs）")
	pf.BoolVar(&enableMask, "mask", true, "控制台/摘要脱敏")
	pf.BoolVar(&noStickyTime, "no-sticky-time", false, "关闭时间戳继承：续行不再沿用上一条时间戳（仅当每行都带时间戳时适用）")
	pf.StringVar(&sinceRel, "since", "", "相对时间窗口，如 30m/2h/7d（与 --start/--end 互斥）")

	root.AddCommand(inspectCmd(), sliceCmd(), grepCmd(), exportCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		exitNow(mapExit(err))
	}
}

func initRuntime() error {
	var ks *crypto.KeyStore
	keyPath := keyFilePath
	var err error
	if keyPath == "" {
		keyPath = filepath.Join(".logseek", "key")
	}
	if configPath != "" || keyFilePath != "" || dirExists(".logseek") {
		ks, err = crypto.NewKeyStore(keyPath)
		if err != nil {
			return errkind.Wrap(errkind.KindRuntime, fmt.Errorf("密钥初始化失败（禁止降级）：%w", err))
		}
	}

	appConfig, err = config.Load(configPath, ks)
	if err != nil {
		return errkind.Wrap(errkind.KindRuntime, err)
	}
	if logLevel != "" {
		appConfig.LogLevel = logLevel
	}
	if logDir != "" {
		appConfig.LogDir = logDir
	}
	if !enableMask {
		appConfig.EnableMask = false
	}

	lg, err := logging.New(logging.Options{
		Level:      appConfig.LogLevel,
		Dir:        appConfig.LogDir,
		Console:    false,
		Module:     "logseek",
		EnableMask: appConfig.EnableMask,
	})
	if err != nil {
		return err
	}
	appLogger = lg
	if appConfig != nil {
		if logLevel != "" {
			appConfig.LogLevel = logLevel
		}
		if logDir != "" {
			appConfig.LogDir = logDir
		}
		warnConfigIssues(appConfig)
	}
	return nil
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// maybeSaveConfig 按 --save-config 落盘当前生效配置。
//
// 口令统一以 ENC 密文写入；--ensure-password 时为空口令生成强口令。
// 无密钥句柄时拒绝保存，防止明文落盘。
func maybeSaveConfig() error {
	if saveConfig == "" {
		return nil
	}
	ks, err := ensureKeyStore()
	if err != nil {
		return err
	}
	if appConfig == nil {
		appConfig = config.Default()
	}
	if err := config.SaveWithOptions(saveConfig, appConfig, ks, config.SaveOptions{
		EnsurePassword: ensurePwd,
	}); err != nil {
		return errkind.Wrap(errkind.KindRuntime, err)
	}
	fmt.Fprintln(os.Stderr, "配置已保存:", saveConfig)
	if ensurePwd && appConfig.Password != "" {
		fmt.Fprintln(os.Stderr, "已生成强口令，请从该配置文件查看并妥善保管")
	}
	return nil
}

// ensureKeyStore 复用当前密钥路径创建/加载密钥句柄。
func ensureKeyStore() (*crypto.KeyStore, error) {
	p := keyFilePath
	if p == "" {
		if appConfig != nil && appConfig.KeyFile != "" {
			p = appConfig.KeyFile
		} else {
			p = filepath.Join(".logseek", "key")
		}
	}
	ks, err := crypto.NewKeyStore(p)
	if err != nil {
		return nil, errkind.Wrap(errkind.KindRuntime, fmt.Errorf("密钥初始化失败（禁止降级）：%w", err))
	}
	return ks, nil
}

// warnConfigIssues 把加载期的安全告警回显给用户。
func warnConfigIssues(cfg *config.Config) {
	if cfg == nil {
		return
	}
	for _, w := range cfg.Warnings {
		fmt.Fprintln(os.Stderr, "安全提示:", w)
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
				logErr(err)
				fmt.Fprintln(os.Stderr, "错误:", err)
				exitNow(mapExit(err))
			}
			if appLogger != nil {
				appLogger.Infof("预检完成 文件=%s 编码=%s 可切片=%v", args[0], rep.Encoding, rep.Sliceable)
			}
			if jsonOut {
				b, _ := json.MarshalIndent(rep, "", "  ")
				fmt.Println(string(b))
			} else {
				fmt.Print(rep.Format())
			}
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
			req, err := buildExportReq(args[0], true, false)
			if err != nil {
				fmt.Fprintln(os.Stderr, "错误:", err)
				exitNow(mapExit(err))
			}
			runExport(req)
			return nil
		},
	}
	bindTimeFlags(c)
	bindOutFlag(c)
	return c
}

func grepCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "grep <文件路径>",
		Short: "内容检索导出（关键词/正则）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(keywords) == 0 && pattern == "" {
				err := errkind.New(errkind.KindUsage, "必须指定至少一个关键词或 --pattern")
				fmt.Fprintln(os.Stderr, "错误:", err)
				exitNow(mapExit(err))
			}
			req, err := buildExportReq(args[0], false, true)
			if err != nil {
				fmt.Fprintln(os.Stderr, "错误:", err)
				exitNow(mapExit(err))
			}
			runExport(req)
			return nil
		},
	}
	bindSearchFlags(c)
	bindOutFlag(c)
	return c
}

func exportCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "export <文件路径>",
		Short: "时间 ∧ 内容组合导出",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			hasTime := timeStart != "" || timeEnd != "" || sinceRel != ""
			hasSearch := len(keywords) > 0 || pattern != ""
			if !hasTime && !hasSearch {
				err := errkind.New(errkind.KindUsage, "export 至少需要时间窗口或检索条件")
				fmt.Fprintln(os.Stderr, "错误:", err)
				exitNow(mapExit(err))
			}
			req, err := buildExportReq(args[0], hasTime, hasSearch)
			if err != nil {
				fmt.Fprintln(os.Stderr, "错误:", err)
				exitNow(mapExit(err))
			}
			runExport(req)
			return nil
		},
	}
	bindTimeFlags(c)
	bindSearchFlags(c)
	bindOutFlag(c)
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

func bindOutFlag(c *cobra.Command) {
	c.Flags().StringVarP(&output, "output", "o", "", "输出结果文件路径（可选）")
}

// sinceRel 由 persistent flag --since 填充
var sinceRel string

func buildExportReq(src string, enableTime, enableSearch bool) (application.ExportRequest, error) {
	req := application.ExportRequest{
		Source:       src,
		Output:       output,
		EnableTime:   enableTime,
		EnableSearch: enableSearch,
		Keywords:     keywords,
		And:          andMatch,
		IgnoreCase:   ignoreCase,
		WholeWord:    wholeWord,
		Pattern:      pattern,
		TimeFormat:   timeFmt,
		ReadRate:     readRate,
		EnableMask:   appConfig == nil || appConfig.EnableMask,
		Logger:       appLogger,

		DisableStickyTime: noStickyTime,
	}
	if sinceRel != "" && enableTime {
		req.Relative = sinceRel
		if timeStart != "" || timeEnd != "" {
			return req, errkind.New(errkind.KindUsage, "--since 与 --start/--end 不能同时使用")
		}
		return req, nil
	}
	if enableTime {
		if timeStart == "" || timeEnd == "" {
			return req, errkind.New(errkind.KindUsage, "必须同时指定 --start 与 --end，或使用 --since")
		}
		st, err := parseTime(timeStart)
		if err != nil {
			return req, err
		}
		en, err := parseTime(timeEnd)
		if err != nil {
			return req, err
		}
		req.StartTime = st
		req.EndTime = en
	}
	return req, nil
}

func parseTime(s string) (time.Time, error) {
	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05.000",
		time.RFC3339,
		"2006-01-02",
	}
	for _, l := range layouts {
		if t, err := time.ParseInLocation(l, strings.TrimSpace(s), time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errkind.New(errkind.KindUsage, "无法解析时间 %q，请使用 2006-01-02 15:04:05", s)
}

func runExport(req application.ExportRequest) {
	sum, err := application.Export(req)
	if err != nil {
		logErr(err)
		fmt.Fprintln(os.Stderr, "错误:", err)
		exitNow(mapExit(err))
	}
	if appLogger != nil {
		appLogger.Infof("导出完成 源=%s 扫描=%d 命中=%d", req.Source, sum.Scanned, sum.Matched)
	}
	fmt.Println(application.FormatSummary(sum, jsonOut, appConfig == nil || appConfig.EnableMask))
	if req.Output != "" {
		fmt.Println("输出文件:", req.Output)
	}
}

func logErr(err error) {
	if appLogger != nil {
		appLogger.Errorf("%s", err.Error())
	}
}

// exitNow 统一退出入口：先冲刷并关闭日志句柄再退出。
//
// 直接调用 os.Exit 会跳过 PersistentPostRun，导致日志缓冲区丢失、句柄不释放，
// 因此所有退出路径都必须走这里。
func exitNow(code int) {
	if appLogger != nil {
		appLogger.Close()
		appLogger = nil
	}
	os.Exit(code)
}

// mapExit 按错误类别映射退出码。
//
// 不依赖错误文本：早期实现靠 strings.Contains 匹配中文文案，
// 一旦文案调整或包装层级变化就会误判，现改由 domain/errkind 承载类别。
func mapExit(err error) int {
	if err == nil {
		return exitOK
	}
	switch errkind.KindOf(err) {
	case errkind.KindUsage:
		return exitUsage
	case errkind.KindNotFound:
		return exitNotFound
	case errkind.KindProbeFailed:
		return exitProbeFailed
	case errkind.KindInterrupt:
		return exitInterrupt
	case errkind.KindRuntime:
		return exitRuntime
	default:
		// 未分类错误统一按运行错误，保持与历史行为一致
		return exitRuntime
	}
}
