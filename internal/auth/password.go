package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

// MinPasswordLen / MaxPasswordLen 定义口令长度边界（bcrypt 只处理前 72 字节）。
const (
	MinPasswordLen = 12
	MaxPasswordLen = 72
)

// ErrPasswordPolicy 表示口令不满足策略。
var ErrPasswordPolicy = errors.New("口令不符合策略")

// HashPassword 使用 bcrypt（自带随机盐）生成口令哈希，禁止明文落库。
func HashPassword(plain string) (string, error) {
	if err := ValidatePassword(plain); err != nil {
		return "", err
	}
	b, err := bcrypt.GenerateFromPassword([]byte(plain), 12)
	if err != nil {
		return "", fmt.Errorf("生成口令哈希失败：%w", err)
	}
	return string(b), nil
}

// ComparePassword 校验口令；哈希格式非法时只返回不匹配，不向调用方暴露细节。
func ComparePassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// ValidatePassword 校验口令策略：长度 12–72 字节、不能全为空白、不能与用户名相同。
func ValidatePassword(plain string) error {
	if strings.TrimSpace(plain) == "" {
		return fmt.Errorf("%w：口令不能为空", ErrPasswordPolicy)
	}
	n := len([]byte(plain))
	if n < MinPasswordLen || n > MaxPasswordLen {
		return fmt.Errorf("%w：长度需在 %d-%d 字节之间（当前 %d 字节）", ErrPasswordPolicy, MinPasswordLen, MaxPasswordLen, n)
	}
	if len(strings.TrimSpace(plain)) < MinPasswordLen {
		return fmt.Errorf("%w：长度需在 %d-%d 字节之间", ErrPasswordPolicy, MinPasswordLen, MaxPasswordLen)
	}
	if utf8.RuneCountInString(plain) == 1 {
		return fmt.Errorf("%w：口令过于简单", ErrPasswordPolicy)
	}
	return nil
}

// HashToken 生成随机会话令牌：返回明文（仅写 Cookie）与 SHA-256 十六进制（仅落库）。
func HashToken() (plain string, hashed string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("生成会话令牌失败：%w", err)
	}
	plain = hex.EncodeToString(b)
	return plain, HashTokenString(plain), nil
}

// HashTokenString 计算令牌哈希。
func HashTokenString(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}
