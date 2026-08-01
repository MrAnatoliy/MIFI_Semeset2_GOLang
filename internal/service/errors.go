package service

import (
	"errors"
	"math"
)

// Ошибки бизнес-логики с понятными сообщениями для клиента API.
var (
	ErrUserExists        = errors.New("пользователь с таким email или username уже зарегистрирован")
	ErrInvalidCredential = errors.New("неверный логин или пароль")
	ErrForbidden         = errors.New("операция с чужим счётом или картой запрещена")
	ErrNotFound          = errors.New("объект не найден")
	ErrInsufficientFunds = errors.New("недостаточно средств на счёте")
	ErrCardExpired       = errors.New("срок действия карты истёк")
	ErrCardBlocked       = errors.New("карта заблокирована")
	ErrInvalidCVV        = errors.New("неверный CVV")
	ErrIntegrity         = errors.New("нарушена целостность данных карты")
	ErrCurrency          = errors.New("поддерживается только валюта RUB")
	ErrPredictRange      = errors.New("период прогноза должен быть от 1 до 365 дней")
)

// round2 округляет денежную величину до копеек.
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
