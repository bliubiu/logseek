// Package logging 实现 AGENTS.md 日志规范：级别、格式、按日轮转、保留 32 天。
package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bliubiu/logseek/internal/infrastructure/mask"
)

// Level 日志级别。
type Level int

const (
	Debug Level = iota
	Info
	Error
)

func (l Level) String() string {
	switch l {
	case Debug:
		return "DEBUG"
	case Error:
		return "ERROR"
	default:
		return "INFO"
	}
}

// ParseLevel 解析级别；非法返回 Info。
func ParseLevel(s string) Level {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "DEBUG":
		return Debug
	case "ERROR":
		return Error
	default:
		return Info
	}
}

// Logger 按 AGENTS 格式输出，支持文件按日轮转与控制台。
type Logger struct {
	mu       sync.Mutex
	level    Level
	dir      string
	file     *os.File
	fileDay  string
	console  bool
	module   string
	retention int
	enableMask bool
}

// Options 构建选项。
type Options struct {
	Level      string
	Dir        string
	Console    bool
	Module     string
	Retention  int // 保留天数，默认 32
	EnableMask bool
}

// New 创建日志器；Dir 非空时写文件。
func New(opt Options) (*Logger, error) {
	if opt.Retention <= 0 {
		opt.Retention = 32
	}
	if opt.Module == "" {
		opt.Module = "logseek"
	}
	l := &Logger{
		level:      ParseLevel(opt.Level),
		dir:        opt.Dir,
		console:    opt.Console,
		module:     opt.Module,
		retention:  opt.Retention,
		enableMask: opt.EnableMask,
	}
	if opt.Dir != "" {
		if err := os.MkdirAll(opt.Dir, 0o755); err != nil {
			return nil, fmt.Errorf("创建日志目录失败：%w", err)
		}
		if err := l.rotateIfNeeded(); err != nil {
			return nil, err
		}
	}
	l.Cleanup()
	return l, nil
}

func (l *Logger) rotateIfNeeded() error {
	day := time.Now().Format("20060102")
	if l.file != nil && l.fileDay == day {
		return nil
	}
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
	name := fmt.Sprintf("logseek-%s.log", day)
	f, err := os.OpenFile(filepath.Join(l.dir, name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("打开日志文件失败：%w", err)
	}
	l.file = f
	l.fileDay = day
	return nil
}

// Cleanup 清理超过保留天数的日志。
func (l *Logger) Cleanup() {
	if l.dir == "" {
		return
	}
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -l.retention)
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "logseek-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		dayStr := strings.TrimSuffix(strings.TrimPrefix(name, "logseek-"), ".log")
		t, err := time.ParseInLocation("20060102", dayStr, time.Local)
		if err != nil {
			continue
		}
		if t.Before(cutoff) {
			_ = os.Remove(filepath.Join(l.dir, name))
		}
	}
}

// Log 记录一条日志；msg 会脱敏。
func (l *Logger) Log(lv Level, msg string) {
	if l == nil || lv < l.level {
		return
	}
	if l.enableMask {
		msg = mask.Apply(msg)
	}
	now := time.Now()
	ts := now.Format("2006-01-02 15:04:05.000")
	pid := os.Getpid()
	line := fmt.Sprintf("[%s] [%s] [%d] [%s] - %s", ts, lv.String(), pid, l.module, msg)

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.dir != "" {
		if err := l.rotateIfNeeded(); err == nil && l.file != nil {
			_, _ = l.file.WriteString(line + "\n")
		}
	}
	if l.console {
		fmt.Fprintln(os.Stderr, line)
	}
}

// Debugf / Infof / Errorf
func (l *Logger) Debugf(format string, args ...any) { l.Log(Debug, fmt.Sprintf(format, args...)) }
func (l *Logger) Infof(format string, args ...any)  { l.Log(Info, fmt.Sprintf(format, args...)) }
func (l *Logger) Errorf(format string, args ...any) { l.Log(Error, fmt.Sprintf(format, args...)) }

// Close 关闭文件。
func (l *Logger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
}

// LogFiles 列出日志文件名（测试用）。
func LogFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".log") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}
