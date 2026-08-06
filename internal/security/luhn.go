package security

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// BIN эмитента (первые цифры номера карты).
const cardBIN = "427601"

func randomDigits(n int) (string, error) {
	var sb strings.Builder
	for i := 0; i < n; i++ {
		d, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		sb.WriteString(d.String())
	}
	return sb.String(), nil
}

// luhnCheckDigit вычисляет контрольную цифру для частичного номера.
func luhnCheckDigit(partial string) int {
	sum := 0
	double := true // следующая позиция справа - удваиваемая
	for i := len(partial) - 1; i >= 0; i-- {
		d := int(partial[i] - '0')
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return (10 - sum%10) % 10
}

// ValidLuhn проверяет номер по алгоритму Луна.
func ValidLuhn(number string) bool {
	if len(number) < 2 {
		return false
	}
	sum := 0
	double := false
	for i := len(number) - 1; i >= 0; i-- {
		c := number[i]
		if c < '0' || c > '9' {
			return false
		}
		d := int(c - '0')
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return sum%10 == 0
}

// GenerateCardNumber выпускает 16-значный номер, валидный по алгоритму Луна.
func GenerateCardNumber() (string, error) {
	body, err := randomDigits(16 - len(cardBIN) - 1)
	if err != nil {
		return "", err
	}
	partial := cardBIN + body
	return fmt.Sprintf("%s%d", partial, luhnCheckDigit(partial)), nil
}

// GenerateCVV возвращает трёхзначный CVV.
func GenerateCVV() (string, error) {
	return randomDigits(3)
}

// GenerateExpiry возвращает срок действия карты в формате MM/YY (+4 года).
func GenerateExpiry() string {
	t := time.Now().AddDate(4, 0, 0)
	return t.Format("01/06")
}

// ExpiryValid проверяет, что срок действия карты не истёк.
func ExpiryValid(expiry string) bool {
	t, err := time.Parse("01/06", expiry)
	if err != nil {
		return false
	}
	// Карта действительна до последнего дня указанного месяца.
	end := t.AddDate(0, 1, 0)
	return time.Now().Before(end)
}

// MaskCardNumber скрывает середину номера карты.
func MaskCardNumber(number string) string {
	if len(number) < 8 {
		return strings.Repeat("*", len(number))
	}
	return number[:4] + " **** **** " + number[len(number)-4:]
}

// GenerateAccountNumber формирует 20-значный номер счёта (формат ЦБ РФ, RUB).
func GenerateAccountNumber() (string, error) {
	tail, err := randomDigits(11)
	if err != nil {
		return "", err
	}
	// 40817 - счёт физлица, 810 - код рубля, 1 - контрольный разряд-заглушка.
	return "40817" + "810" + "1" + tail, nil
}
