// Package crypto 实现 00-安全设计 规定的 ENC() 加解密。
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// EncPrefix ENC() 外层标识。
	EncPrefix = "ENC("
	EncSuffix = ")"

	// 算法标签
	algoAES256GCM = 0x00
	algoSM4GCM    = 0x01

	nonceLen  = 12
	tagLen    = 16
	keyLen    = 32
	prefixLen = len(EncPrefix)
)

// KeyStore 密钥存取；Unix 权限 0600，失败即终止语义由调用方处理。
type KeyStore struct {
	path string
	key  []byte
}

// NewKeyStore 加载或初始化密钥文件（32 字节随机）。
func NewKeyStore(path string) (*KeyStore, error) {
	if path == "" {
		return nil, fmt.Errorf("密钥路径不能为空")
	}
	if b, err := os.ReadFile(path); err == nil {
		if len(b) != keyLen {
			return nil, fmt.Errorf("密钥长度非法，期望 %d 字节", keyLen)
		}
		return &KeyStore{path: path, key: b}, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("读取密钥失败：%w", err)
	}

	// 首次生成
	key := make([]byte, keyLen)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("生成密钥失败：%w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("创建密钥目录失败：%w", err)
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, fmt.Errorf("写入密钥失败：%w", err)
	}
	// Windows 上 chmod 语义有限，仍尽力设置。
	_ = os.Chmod(path, 0o600)
	return &KeyStore{path: path, key: key}, nil
}

// Key 返回密钥副本。
func (k *KeyStore) Key() []byte {
	out := make([]byte, len(k.key))
	copy(out, k.key)
	return out
}

// Encrypt 明文 → ENC(base64(algo||nonce||ct||tag))。
func Encrypt(key, plain []byte) (string, error) {
	if len(key) != keyLen {
		return "", fmt.Errorf("密钥长度非法")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	// sealed = ct || tag
	sealed := gcm.Seal(nil, nonce, plain, nil)
	payload := make([]byte, 0, 1+nonceLen+len(sealed))
	payload = append(payload, algoAES256GCM)
	payload = append(payload, nonce...)
	payload = append(payload, sealed...)
	return EncPrefix + base64.StdEncoding.EncodeToString(payload) + EncSuffix, nil
}

// Decrypt ENC(...) → 明文；非 ENC 格式原样返回。
func Decrypt(key []byte, s string) ([]byte, error) {
	if !strings.HasPrefix(s, EncPrefix) || !strings.HasSuffix(s, EncSuffix) {
		return []byte(s), nil
	}
	inner := s[prefixLen : len(s)-1]
	raw, err := base64.StdEncoding.DecodeString(inner)
	if err != nil {
		return nil, fmt.Errorf("密文 Base64 解码失败")
	}
	if len(raw) < 1+nonceLen+tagLen {
		return nil, fmt.Errorf("密文格式非法")
	}
	algo := raw[0]
	nonce := raw[1 : 1+nonceLen]
	ct := raw[1+nonceLen:]
	switch algo {
	case algoAES256GCM:
		if len(key) != keyLen {
			return nil, fmt.Errorf("密钥长度非法")
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, err
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return nil, err
		}
		plain, err := gcm.Open(nil, nonce, ct, nil)
		if err != nil {
			return nil, fmt.Errorf("密文校验失败，可能已被篡改")
		}
		return plain, nil
	case algoSM4GCM:
		return nil, fmt.Errorf("暂不支持的加密算法标签 0x01")
	default:
		return nil, fmt.Errorf("未知算法标签：0x%02x", algo)
	}
}

// IsEncrypted 是否已是 ENC 格式。
func IsEncrypted(s string) bool {
	return strings.HasPrefix(s, EncPrefix) && strings.HasSuffix(s, EncSuffix)
}

// passwordAlphabet 强口令字符表：大小写字母 + 数字 + 少量安全特殊字符，
// 去掉引号与反斜杠，避免写进 JSON/配置文件时需要转义。
const passwordAlphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789!@#$%^&*-_=+"

// RandomPassword 生成 n 字符强口令（密码学随机）。
//
// 用于安全设计 2.5「空口令自动生成强口令」，禁止使用 math/rand，
// 且刻意排除易混淆字符（l/I/1、O/0）。
func RandomPassword(n int) (string, error) {
	if n <= 0 {
		n = 32
	}
	out := make([]byte, n)
	max := uint64(256) / uint64(len(passwordAlphabet)) * uint64(len(passwordAlphabet))
	for i := range out {
		var b byte
		// 拒绝采样避免取模偏斜
		for {
			var buf [1]byte
			if _, err := rand.Read(buf[:]); err != nil {
				return "", fmt.Errorf("生成随机口令失败：%w", err)
			}
			if uint64(buf[0]) < max {
				b = buf[0]
				break
			}
		}
		out[i] = passwordAlphabet[int(b)%len(passwordAlphabet)]
	}
	return string(out), nil
}
