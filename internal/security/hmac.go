package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// HMACSigner формирует и проверяет HMAC-SHA256 подписи данных карт.
type HMACSigner struct {
	secret []byte
}

// NewHMACSigner создаёт подписанта на общем секрете.
func NewHMACSigner(secret string) *HMACSigner {
	return &HMACSigner{secret: []byte(secret)}
}

// Compute возвращает hex-представление HMAC-SHA256 от данных.
func (s *HMACSigner) Compute(data string) string {
	h := hmac.New(sha256.New, s.secret)
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

// Verify выполняет сравнение подписей в постоянном времени.
func (s *HMACSigner) Verify(data, signature string) bool {
	expected := s.Compute(data)
	return hmac.Equal([]byte(expected), []byte(signature))
}
