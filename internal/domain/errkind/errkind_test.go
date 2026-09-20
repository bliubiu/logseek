package errkind_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/bliubiu/logseek/internal/domain/errkind"
)

func TestNewAndKindOf(t *testing.T) {
	err := errkind.New(errkind.KindUsage, "缺少参数 %s", "--start")
	if err.Error() != "缺少参数 --start" {
		t.Fatalf("Error() = %q", err.Error())
	}
	if got := errkind.KindOf(err); got != errkind.KindUsage {
		t.Fatalf("KindOf = %v, 期望 usage", got)
	}
	if errkind.KindOf(nil) != errkind.KindUnknown {
		t.Fatal("nil 错误应为 unknown")
	}
}

func TestKindOfThroughWrappingChain(t *testing.T) {
	// 关键：包装多层后仍能取到类别（errors.As 语义）
	base := errkind.New(errkind.KindNotFound, "文件不存在：a.log")
	wrapped := fmt.Errorf("导出失败：%w", base)
	wrapped2 := fmt.Errorf("外层再包一层：%w", wrapped)
	if got := errkind.KindOf(wrapped2); got != errkind.KindNotFound {
		t.Fatalf("多层包装后 KindOf = %v, 期望 not-found", got)
	}
}

func TestWrapKeepsOriginalKind(t *testing.T) {
	inner := errors.New("底层故障")
	err := errkind.Wrap(errkind.KindRuntime, inner)
	if err.Error() != "底层故障" {
		t.Fatalf("Wrap 后文案应保持原样: %q", err.Error())
	}
	if got := errkind.KindOf(err); got != errkind.KindRuntime {
		t.Fatalf("KindOf = %v, 期望 runtime", got)
	}
	// 已分类的错误再包一层不应被降级覆盖
	again := errkind.Wrap(errkind.KindUsage, err)
	if got := errkind.KindOf(again); got != errkind.KindRuntime {
		t.Fatalf("已分类错误被覆盖: %v", got)
	}
	if !errors.Is(again, inner) {
		t.Fatal("errors.Is 断链")
	}
}

func TestWrapNil(t *testing.T) {
	if err := errkind.Wrap(errkind.KindRuntime, nil); err != nil {
		t.Fatalf("Wrap(nil) 应返回 nil, got %v", err)
	}
}

func TestKindString(t *testing.T) {
	cases := map[errkind.Kind]string{
		errkind.KindUnknown:     "unknown",
		errkind.KindUsage:       "usage",
		errkind.KindNotFound:    "not-found",
		errkind.KindProbeFailed: "probe-failed",
		errkind.KindRuntime:     "runtime",
		errkind.KindInterrupt:   "interrupt",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("Kind(%d).String() = %q, 期望 %q", k, got, want)
		}
	}
}

func TestNonClassifiedErrorIsUnknown(t *testing.T) {
	if got := errkind.KindOf(errors.New("任意错误")); got != errkind.KindUnknown {
		t.Fatalf("未分类错误 = %v, 期望 unknown", got)
	}
}
