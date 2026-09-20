// Package config 加载配置并自动解密 ENC 敏感字段。
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bliubiu/logseek/internal/infrastructure/crypto"
)

// Config 应用配置。
type Config struct {
	// LogLevel 日志级别 DEBUG/INFO/ERROR，默认 INFO
	LogLevel string `json:"log_level"`
	// LogDir 日志目录，默认 logs/
	LogDir string `json:"log_dir"`
	// KeyFile 密钥文件路径
	KeyFile string `json:"key_file"`
	// Password 敏感字段示例（ENC 存储）
	Password string `json:"password"`
	// DefaultBlockSize 默认读块
	DefaultBlockSize int `json:"default_block_size"`
	// EnableMask 是否对控制台/摘要脱敏，默认 true
	EnableMask bool `json:"enable_mask"`
	// Warnings 加载期产生的非致命安全告警（不落盘、不进 JSON）。
	Warnings []string `json:"-"`
}

// Default 返回默认配置。
func Default() *Config {
	return &Config{
		LogLevel:         "INFO",
		LogDir:           "logs",
		KeyFile:          filepath.Join(".logseek", "key"),
		Password:         "",
		DefaultBlockSize: 1 << 20,
		EnableMask:       true,
	}
}

// Load 读取 JSON 配置；path 为空返回默认。加载后解密敏感字段。
// 密钥异常直接返回错误（调用方必须终止，不降级）。
func Load(path string, ks *crypto.KeyStore) (*Config, error) {
	cfg := Default()
	if path == "" {
		return cfg, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("配置文件不存在：%s", path)
		}
		return nil, fmt.Errorf("读取配置失败：%w", err)
	}
	if err := json.Unmarshal(b, cfg); err != nil {
		return nil, fmt.Errorf("配置文件格式非法：%w", err)
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "INFO"
	}
	if cfg.LogDir == "" {
		cfg.LogDir = "logs"
	}
	if cfg.DefaultBlockSize <= 0 {
		cfg.DefaultBlockSize = 1 << 20
	}

	// 解密敏感字段
	if ks != nil && cfg.Password != "" {
		if crypto.IsEncrypted(cfg.Password) {
			plain, err := crypto.Decrypt(ks.Key(), cfg.Password)
			if err != nil {
				return nil, fmt.Errorf("配置敏感字段解密失败：%w", err)
			}
			// 仅驻留内存；禁止写入日志
			cfg.Password = string(plain)
		} else {
			// 安全设计 3.3：禁止明文存储口令。此处不阻断运行，改为显式告警，
			// 调用方应把告警呈现给用户而不是静默放行。
			cfg.Warnings = append(cfg.Warnings,
				"检测到配置中的口令为明文存储，请使用 --save-config 重新落盘以加密保存")
		}
	}
	return cfg, nil
}

// Save 将配置以 JSON 落盘；Password 在写盘前加密为 ENC 格式。
//
// 写盘语义：
//   - 已是 ENC 密文：原样落盘，避免重复加密；
//   - 明文：加密后落盘，内存中的明文不被篡改；
//   - 空且 ensurePassword：先生成强口令写入 cfg 再落盘（安全设计 2.5）。
//
// 文件权限 0600：配置可能包含口令密文与其他敏感项；Windows 上尽力设置。
func Save(path string, cfg *Config, ks *crypto.KeyStore) error {
	return SaveWithOptions(path, cfg, ks, SaveOptions{})
}

// SaveOptions 保存行为选项。
type SaveOptions struct {
	// EnsurePassword 口令为空时自动生成强口令（不再为空）。
	EnsurePassword bool
	// PasswordLength 强口令长度，0 表示默认 32。
	PasswordLength int
}

// SaveWithOptions 按选项保存配置。
func SaveWithOptions(path string, cfg *Config, ks *crypto.KeyStore, o SaveOptions) error {
	if path == "" {
		return fmt.Errorf("保存路径不能为空")
	}
	if cfg == nil {
		return fmt.Errorf("配置对象为空")
	}
	if ks == nil {
		return fmt.Errorf("缺少密钥句柄，拒绝以明文保存口令")
	}
	if o.EnsurePassword && cfg.Password == "" {
		pwd, err := crypto.RandomPassword(o.PasswordLength)
		if err != nil {
			return err
		}
		cfg.Password = pwd
	}
	// 序列化副本：口令替换为密文，避免污染内存中的明文配置
	disk := *cfg
	disk.Warnings = nil
	if cfg.Password != "" {
		enc, err := EncryptPassword(ks, cfg.Password)
		if err != nil {
			return fmt.Errorf("口令加密失败：%w", err)
		}
		disk.Password = enc
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("创建配置目录失败：%w", err)
	}
	b, err := json.MarshalIndent(&disk, "", "  ")
	if err != nil {
		return fmt.Errorf("配置序列化失败：%w", err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return fmt.Errorf("写入配置文件失败：%w", err)
	}
	_ = os.Chmod(path, 0o600)
	return nil
}

// GeneratePassword 生成强口令（安全设计 2.5 空口令场景）。
func GeneratePassword(n int) (string, error) { return crypto.RandomPassword(n) }

// EncryptPassword 将明文密码加密为 ENC 格式（写配置用）。
func EncryptPassword(ks *crypto.KeyStore, plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	if crypto.IsEncrypted(plain) {
		return plain, nil
	}
	return crypto.Encrypt(ks.Key(), []byte(plain))
}
