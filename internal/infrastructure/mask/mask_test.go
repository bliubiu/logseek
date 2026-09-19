package mask_test

import (
	"strings"
	"testing"

	"github.com/bliubiu/logseek/internal/infrastructure/mask"
)

func TestIPv4(t *testing.T) {
	got := mask.Apply("from 192.168.10.3 ok")
	if got != "from *.*.10.3 ok" {
		t.Fatalf("got %s", got)
	}
	if mask.Apply("local 127.0.0.1") != "local 127.0.0.1" {
		t.Fatal("回环应保留")
	}
}

func TestPhoneAndID(t *testing.T) {
	got := mask.Apply("手机13812345678 身份证110101199001011234")
	if strings.Contains(got, "13812345678") {
		t.Fatal("手机号未脱敏")
	}
	if strings.Contains(got, "110101199001011234") {
		t.Fatal("身份证未脱敏")
	}
}

func TestEmail(t *testing.T) {
	got := mask.Apply("联系 user@example.com")
	if strings.Contains(got, "user@example.com") {
		t.Fatal("邮箱未脱敏")
	}
}

func TestMaskWriter(t *testing.T) {
	var buf strings.Builder
	w := mask.MaskWriter{W: writerFunc(func(p []byte) (int, error) {
		return buf.Write(p)
	})}
	if _, err := w.Write([]byte("ip 10.0.0.1")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "*.*.0.1") {
		t.Fatalf("writer 未脱敏: %s", buf.String())
	}
}

type writerFunc func(p []byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }
