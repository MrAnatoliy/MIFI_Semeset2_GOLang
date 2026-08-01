package repository

import (
	"context"
	"database/sql"

	"bankapi/internal/models"
)

// UserRepository инкапсулирует SQL-запросы к таблице users.
type UserRepository struct {
	db *sql.DB
}

// Create сохраняет нового пользователя. При конфликте по email/username
// возвращает ErrDuplicate.
func (r *UserRepository) Create(ctx context.Context, u *models.User) error {
	const q = `
		INSERT INTO users (username, email, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, created_at`
	err := r.db.QueryRowContext(ctx, q, u.Username, u.Email, u.PasswordHash).
		Scan(&u.ID, &u.CreatedAt)
	return mapDBError(err)
}

// GetByLogin ищет пользователя по email или username.
func (r *UserRepository) GetByLogin(ctx context.Context, login string) (*models.User, error) {
	const q = `
		SELECT id, username, email, password_hash, created_at
		FROM users
		WHERE email = lower($1) OR username = $1`
	u := &models.User{}
	err := r.db.QueryRowContext(ctx, q, login).
		Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return nil, mapDBError(err)
	}
	return u, nil
}

// GetByID возвращает пользователя по идентификатору.
func (r *UserRepository) GetByID(ctx context.Context, id int64) (*models.User, error) {
	const q = `
		SELECT id, username, email, password_hash, created_at
		FROM users WHERE id = $1`
	u := &models.User{}
	err := r.db.QueryRowContext(ctx, q, id).
		Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return nil, mapDBError(err)
	}
	return u, nil
}

// Exists проверяет уникальность email и username до вставки.
func (r *UserRepository) Exists(ctx context.Context, email, username string) (bool, error) {
	const q = `SELECT EXISTS(SELECT 1 FROM users WHERE email = $1 OR username = $2)`
	var exists bool
	if err := r.db.QueryRowContext(ctx, q, email, username).Scan(&exists); err != nil {
		return false, mapDBError(err)
	}
	return exists, nil
}
