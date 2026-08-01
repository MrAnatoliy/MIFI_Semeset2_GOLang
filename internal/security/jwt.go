package security

import (
	"errors"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ErrInvalidToken возвращается при неуспешной проверке JWT.
var ErrInvalidToken = errors.New("invalid token")

// TokenManager выпускает и проверяет JWT-токены доступа.
type TokenManager struct {
	secret []byte
	ttl    time.Duration
}

// NewTokenManager создаёт менеджер токенов.
func NewTokenManager(secret string, ttl time.Duration) *TokenManager {
	return &TokenManager{secret: []byte(secret), ttl: ttl}
}

// TTL возвращает срок жизни токена.
func (m *TokenManager) TTL() time.Duration { return m.ttl }

// Generate выпускает токен для пользователя (срок действия — 24 часа по умолчанию).
func (m *TokenManager) Generate(userID int64, email string) (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Subject:   strconv.FormatInt(userID, 10),
		Audience:  jwt.ClaimStrings{email},
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
		Issuer:    "bank-api",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

// Parse проверяет подпись и срок действия токена, возвращая ID пользователя.
func (m *TokenManager) Parse(tokenString string) (int64, error) {
	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return m.secret, nil
	}, jwt.WithIssuer("bank-api"), jwt.WithValidMethods([]string{"HS256"}))

	if err != nil || !token.Valid {
		return 0, ErrInvalidToken
	}
	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil {
		return 0, ErrInvalidToken
	}
	return userID, nil
}
