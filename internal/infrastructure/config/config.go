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
		plain, err := crypto.Decrypt(ks.Key(), cfg.Password)
		if err != nil {
			return nil, fmt.Errorf("配置敏感字段解密失败：%w", err)
		}
		// 仅驻留内存；禁止写入日志
		cfg.Password = string(plain)
	}
	return cfg, nil
}

// EncryptPassword 将明文密码加密为 ENC 格式（写配置用）。
func EncryptPassword(ks *crypto.KeyStore, plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	return crypto.Encrypt(ks.Key(), []byte(plain))
}
