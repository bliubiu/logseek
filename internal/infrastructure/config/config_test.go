package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bliubiu/logseek/internal/infrastructure/config"
	"github.com/bliubiu/logseek/internal/infrastructure/crypto"
)

func TestLoadDefault(t *testing.T) {
	cfg, err := config.Load("", nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LogLevel != "INFO" || !cfg.EnableMask {
		t.Fatalf("%+v", cfg)
	}
}

func TestLoadMissing(t *testing.T) {
	_, err := config.Load(filepath.Join(t.TempDir(), "no.json"), nil)
	if err == nil {
		t.Fatal("缺失应报错")
	}
}

func TestLoadWithEncPassword(t *testing.T) {
	dir := t.TempDir()
	ks, err := crypto.NewKeyStore(filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	enc, err := config.EncryptPassword(ks, "S3cret!")
	if err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, "config.json")
	body := `{"log_level":"DEBUG","password":"` + enc + `"}`
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfgPath, ks)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Password != "S3cret!" {
		t.Fatalf("解密失败: %q", cfg.Password)
	}
	if cfg.LogLevel != "DEBUG" {
		t.Fatal("log_level 未加载")
	}
}

func TestLoadBadKeyFails(t *testing.T) {
	dir := t.TempDir()
	// 坏密钥
	if err := os.WriteFile(filepath.Join(dir, "key"), []byte("xx"), 0o600); err != nil {
		t.Fatal(err)
	}
	ks, err := crypto.NewKeyStore(filepath.Join(dir, "key"))
	if err == nil {
		// 长度非 32 应在 NewKeyStore 失败
		t.Fatal("坏密钥应失败")
	}
	_ = ks
}
