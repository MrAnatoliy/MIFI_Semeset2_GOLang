package service

import (
	"context"
	"errors"

	"github.com/sirupsen/logrus"

	"bankapi/internal/models"
	"bankapi/internal/repository"
	"bankapi/internal/security"
)

// AuthService реализует регистрацию и аутентификацию пользователей.
type AuthService struct {
	users  *repository.UserRepository
	tokens *security.TokenManager
	mailer *Mailer
	log    *logrus.Logger
}

// NewAuthService создаёт сервис аутентификации.
func NewAuthService(users *repository.UserRepository, tokens *security.TokenManager,
	mailer *Mailer, log *logrus.Logger) *AuthService {
	return &AuthService{users: users, tokens: tokens, mailer: mailer, log: log}
}

// Register создаёт пользователя с проверкой уникальности email и username.
func (s *AuthService) Register(ctx context.Context, req *models.RegisterRequest) (*models.AuthResponse, error) {
	exists, err := s.users.Exists(ctx, req.Email, req.Username)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrUserExists
	}

	hash, err := security.HashPassword(req.Password)
	if err != nil {
		return nil, err
	}
	user := &models.User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: hash,
	}
	if err := s.users.Create(ctx, user); err != nil {
		if errors.Is(err, repository.ErrDuplicate) {
			return nil, ErrUserExists
		}
		return nil, err
	}
	s.log.Infof("зарегистрирован пользователь id=%d username=%s", user.ID, user.Username)
	s.mailer.NotifyWelcome(user.Email, user.Username)

	return s.issue(user)
}

// Login проверяет учётные данные и выдаёт JWT сроком на 24 часа.
func (s *AuthService) Login(ctx context.Context, req *models.LoginRequest) (*models.AuthResponse, error) {
	user, err := s.users.GetByLogin(ctx, req.Login)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			// Одинаковый ответ, чтобы не раскрывать существование учётной записи.
			return nil, ErrInvalidCredential
		}
		return nil, err
	}
	if !security.CheckPassword(user.PasswordHash, req.Password) {
		s.log.Warnf("неуспешная попытка входа для login=%s", req.Login)
		return nil, ErrInvalidCredential
	}
	return s.issue(user)
}

func (s *AuthService) issue(user *models.User) (*models.AuthResponse, error) {
	token, err := s.tokens.Generate(user.ID, user.Email)
	if err != nil {
		return nil, err
	}
	return &models.AuthResponse{
		Token:     token,
		ExpiresIn: int64(s.tokens.TTL().Seconds()),
		User:      user,
	}, nil
}

// GetUser возвращает пользователя по ID (используется для уведомлений).
func (s *AuthService) GetUser(ctx context.Context, id int64) (*models.User, error) {
	u, err := s.users.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return u, nil
}
