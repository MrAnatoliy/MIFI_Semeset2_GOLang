package service

import (
	"github.com/sirupsen/logrus"

	"bankapi/internal/config"
	"bankapi/internal/repository"
	"bankapi/internal/security"
)

// Services агрегирует все сервисы бизнес-логики приложения.
type Services struct {
	Auth      *AuthService
	Account   *AccountService
	Card      *CardService
	Credit    *CreditService
	Analytics *AnalyticsService
	CBR       *CBRService
	Mailer    *Mailer
	Tokens    *security.TokenManager
}

// New собирает граф зависимостей сервисного слоя.
func New(repos *repository.Repositories, cfg *config.Config, log *logrus.Logger) *Services {
	tokens := security.NewTokenManager(cfg.JWTSecret, cfg.JWTTTL)
	pgp := security.NewPGPCipher(cfg.PGPPassphrase)
	signer := security.NewHMACSigner(cfg.HMACSecret)

	mailer := NewMailer(cfg, log)
	cbr := NewCBRService(cfg, log)

	return &Services{
		Auth:      NewAuthService(repos.User, tokens, mailer, log),
		Account:   NewAccountService(repos, mailer, log),
		Card:      NewCardService(repos, pgp, signer, mailer, log),
		Credit:    NewCreditService(repos, cbr, mailer, cfg, log),
		Analytics: NewAnalyticsService(repos, cfg, log),
		CBR:       cbr,
		Mailer:    mailer,
		Tokens:    tokens,
	}
}
