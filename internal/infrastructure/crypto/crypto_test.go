package crypto_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bliubiu/logseek/internal/infrastructure/crypto"
)

func TestEncryptDecryptRoundtrip(t *testing.T) {
	dir := t.TempDir()
	ks, err := crypto.NewKeyStore(filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	enc, err := crypto.Encrypt(ks.Key(), []byte("超级密码#1"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(enc, "ENC(") {
		t.Fatalf("格式错误: %s", enc)
	}
	plain, err := crypto.Decrypt(ks.Key(), enc)
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != "超级密码#1" {
		t.Fatalf("解密不匹配: %s", plain)
	}
}

func TestDecryptPlainPassthrough(t *testing.T) {
	plain, err := crypto.Decrypt(nil, "not-encrypted")
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != "not-encrypted" {
		t.Fatal("非 ENC 应原样返回")
	}
}

func TestTamperFails(t *testing.T) {
	dir := t.TempDir()
	ks, _ := crypto.NewKeyStore(filepath.Join(dir, "key"))
	enc, _ := crypto.Encrypt(ks.Key(), []byte("hello"))
	// 篡改 Base64 载荷
	inner := enc[4 : len(enc)-1]
	bad := []byte(inner)
	if bad[3] == 'A' {
		bad[3] = 'B'
	} else {
		bad[3] = 'A'
	}
	_, err := crypto.Decrypt(ks.Key(), "ENC("+string(bad)+")")
	if err == nil {
		// 可能 Base64 失败或 tag 失败，都算成功拦截
		t.Fatal("篡改应失败")
	}
}

func TestKeyReload(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "key")
	ks1, err := crypto.NewKeyStore(p)
	if err != nil {
		t.Fatal(err)
	}
	enc, _ := crypto.Encrypt(ks1.Key(), []byte("x"))
	ks2, err := crypto.NewKeyStore(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := crypto.Decrypt(ks2.Key(), enc); err != nil {
		t.Fatalf("重载密钥后应能解密: %v", err)
	}
}

func TestBadKeyLength(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "key")
	if err := os.WriteFile(p, []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := crypto.NewKeyStore(p); err == nil {
		t.Fatal("密钥长度非法应报错")
	}
}
