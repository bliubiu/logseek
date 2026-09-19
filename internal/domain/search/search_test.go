package search_test

import (
	"strings"
	"testing"
	"time"

	"github.com/bliubiu/logseek/internal/domain/search"
)

func TestByteScanKeyword(t *testing.T) {
	m, err := search.Build(search.Condition{Keywords: []string{"ERROR"}, CaseSensitive: true})
	if err != nil {
		t.Fatal(err)
	}
	if !m.Match([]byte("x ERROR y")) {
		t.Error("应命中 ERROR")
	}
	if m.Match([]byte("x error y")) {
		t.Error("大小写敏感时不应命中")
	}
}

func TestCaseInsensitive(t *testing.T) {
	m, _ := search.Build(search.Condition{Keywords: []string{"error"}, CaseSensitive: false})
	if !m.Match([]byte("ERROR here")) {
		t.Error("忽略大小写应命中")
	}
}

func TestMultiKeywordOrAnd(t *testing.T) {
	orM, _ := search.Build(search.Condition{Keywords: []string{"foo", "bar"}, Combine: search.CombineOr})
	andM, _ := search.Build(search.Condition{Keywords: []string{"foo", "bar"}, Combine: search.CombineAnd})

	if !orM.Match([]byte("only foo")) {
		t.Error("OR 应命中单个词")
	}
	if orM.Match([]byte("neither")) {
		t.Error("OR 不应误命中")
	}
	if andM.Match([]byte("only foo")) {
		t.Error("AND 不应只命中一个词")
	}
	if !andM.Match([]byte("foo and bar")) {
		t.Error("AND 应命中两词")
	}
}

func TestRE2Pattern(t *testing.T) {
	m, err := search.Build(search.Condition{Pattern: `conn(?:ection)?\s+refused`, CaseSensitive: false})
	if err != nil {
		t.Fatal(err)
	}
	if !m.Match([]byte("Connection refused by peer")) {
		t.Error("RE2 应命中")
	}
	if m.Match([]byte("all good")) {
		t.Error("RE2 不应误命中")
	}
}

func TestInvalidRE2(t *testing.T) {
	_, err := search.Build(search.Condition{Pattern: `(`})
	if err == nil {
		t.Fatal("非法正则应报错")
	}
	if !strings.Contains(err.Error(), "正则") {
		t.Errorf("应中文提示: %v", err)
	}
}

// 对抗样例：在 RE2 路径下应快速返回，不出现回溯型指数爆炸。
func TestAdversarialNoHang(t *testing.T) {
	m, err := search.Build(search.Condition{Pattern: `(a+)+b`, CaseSensitive: true})
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(strings.Repeat("a", 40))
	done := make(chan bool, 1)
	go func() {
		_ = m.Match(input)
		done <- true
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("匹配疑似回溯卡死")
	}
}
