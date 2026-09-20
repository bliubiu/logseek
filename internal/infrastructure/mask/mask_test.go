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

func TestIPv4VersionNotMasked(t *testing.T) {
	cases := map[string]string{
		// 版本号形态：后随数值段
		"Release 11.2.0.4.0 - 64bit": "Release 11.2.0.4.0 - 64bit",
		// 版本语境词前置
		"v11.2.0.4 build 3":     "v11.2.0.4 build 3",
		"Version 10.2.0.4 done": "Version 10.2.0.4 done",
		// 非法 IPv4：段值越界与前导零
		"metric 999.1.1.1 x": "metric 999.1.1.1 x",
		"seq 010.001.1.1 y":  "seq 010.001.1.1 y",
	}
	for in, want := range cases {
		if got := mask.Apply(in); got != want {
			t.Errorf("Apply(%q) = %q, 期望 %q", in, got, want)
		}
	}
}

func TestIPv4StillMaskedForRealIPs(t *testing.T) {
	got := mask.Apply("conn 10.0.0.1 -> 192.168.10.3 ok")
	if !strings.Contains(got, "*.*.0.1") || !strings.Contains(got, "*.*.10.3") {
		t.Fatalf("真实 IP 未脱敏（相邻多 IP 场景）: %s", got)
	}
}
