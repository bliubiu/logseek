package config_test

import (
	"os"
	"path/filepath"
	"strings"
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

func TestSaveRoundtripEncryptsPassword(t *testing.T) {
	dir := t.TempDir()
	ks, err := crypto.NewKeyStore(filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Password = "S3cret!"
	cfgPath := filepath.Join(dir, "sub", "config.json")
	if err := config.Save(cfgPath, cfg, ks); err != nil {
		t.Fatal(err)
	}
	// 落盘内容必须是密文，不能出现明文口令
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "S3cret!") {
		t.Fatalf("口令明文落盘: %s", raw)
	}
	if !strings.Contains(string(raw), "ENC(") {
		t.Fatalf("未使用 ENC 封装: %s", raw)
	}
	// 内存中仍是明文，未被加密过程污染
	if cfg.Password != "S3cret!" {
		t.Fatalf("内存口令被改写: %q", cfg.Password)
	}
	back, err := config.Load(cfgPath, ks)
	if err != nil {
		t.Fatal(err)
	}
	if back.Password != "S3cret!" {
		t.Fatalf("回读解密失败: %q", back.Password)
	}
}

func TestSaveGeneratesStrongPasswordWhenAsked(t *testing.T) {
	dir := t.TempDir()
	ks, err := crypto.NewKeyStore(filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfgPath := filepath.Join(dir, "config.json")
	if err := config.SaveWithOptions(cfgPath, cfg, ks, config.SaveOptions{
		EnsurePassword: true, PasswordLength: 24,
	}); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Password) != 24 {
		t.Fatalf("强口令长度 = %d, 期望 24", len(cfg.Password))
	}
	raw, _ := os.ReadFile(cfgPath)
	if strings.Contains(string(raw), cfg.Password) {
		t.Fatal("强口令以明文出现在配置文件中")
	}
	back, err := config.Load(cfgPath, ks)
	if err != nil {
		t.Fatal(err)
	}
	if back.Password != cfg.Password {
		t.Fatal("回读口令不一致")
	}
}

func TestSaveWithoutEnsureKeepsEmptyPassword(t *testing.T) {
	dir := t.TempDir()
	ks, err := crypto.NewKeyStore(filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfgPath := filepath.Join(dir, "config.json")
	if err := config.Save(cfgPath, cfg, ks); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(cfgPath)
	if strings.Contains(string(raw), "ENC(") {
		t.Fatal("空口令不应生成密文")
	}
}

func TestSaveRejectsMissingKeyStore(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Password = "plain"
	if err := config.Save(filepath.Join(dir, "c.json"), cfg, nil); err == nil {
		t.Fatal("无密钥句柄时必须拒绝保存，防止明文落盘")
	}
}

func TestLoadWarnsOnPlainTextPassword(t *testing.T) {
	dir := t.TempDir()
	ks, err := crypto.NewKeyStore(filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"password":"Plain123"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfgPath, ks)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Password != "Plain123" {
		t.Fatalf("明文口令应按原样加载: %q", cfg.Password)
	}
	if len(cfg.Warnings) == 0 {
		t.Fatal("明文口令必须给出告警，不能静默放行")
	}
}

func TestLoadEncryptedPasswordHasNoWarning(t *testing.T) {
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
	body := `{"password":"` + enc + `"}`
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfgPath, ks)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Warnings) != 0 {
		t.Fatalf("密文口令不应产生告警: %v", cfg.Warnings)
	}
}

func TestEncryptPasswordIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	ks, err := crypto.NewKeyStore(filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	enc, err := config.EncryptPassword(ks, "S3cret!")
	if err != nil {
		t.Fatal(err)
	}
	again, err := config.EncryptPassword(ks, enc)
	if err != nil {
		t.Fatal(err)
	}
	if enc != again {
		t.Fatal("已是密文时不应重复加密")
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
