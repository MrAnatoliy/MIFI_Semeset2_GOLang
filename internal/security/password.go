package security

import (
	"golang.org/x/crypto/bcrypt"
)

// HashPassword хеширует пароль пользователя bcrypt-ом.
func HashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// CheckPassword сверяет пароль с сохранённым хешем.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// HashCVV хеширует CVV карты (хранение в открытом виде запрещено).
func HashCVV(cvv string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(cvv), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// CheckCVV проверяет CVV при подтверждении операции по карте.
func CheckCVV(hash, cvv string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(cvv)) == nil
}
