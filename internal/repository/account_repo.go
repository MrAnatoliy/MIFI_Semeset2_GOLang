package repository

import (
	"context"
	"database/sql"

	"bankapi/internal/models"
)

// AccountRepository инкапсулирует SQL-запросы к таблице accounts.
type AccountRepository struct {
	db *sql.DB
}

func (r *AccountRepository) q(q Querier) Querier {
	if q == nil {
		return r.db
	}
	return q
}

// Create создаёт счёт с нулевым балансом.
func (r *AccountRepository) Create(ctx context.Context, a *models.Account) error {
	const query = `
		INSERT INTO accounts (user_id, number, balance, currency)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`
	err := r.db.QueryRowContext(ctx, query, a.UserID, a.Number, a.Balance, a.Currency).
		Scan(&a.ID, &a.CreatedAt)
	return mapDBError(err)
}

// GetByID возвращает счёт по идентификатору.
func (r *AccountRepository) GetByID(ctx context.Context, q Querier, id int64) (*models.Account, error) {
	const query = `
		SELECT id, user_id, number, balance::float8, currency, created_at
		FROM accounts WHERE id = $1`
	return r.scanOne(ctx, r.q(q), query, id)
}

// GetForUpdate блокирует строку счёта до конца транзакции (SELECT ... FOR UPDATE).
func (r *AccountRepository) GetForUpdate(ctx context.Context, q Querier, id int64) (*models.Account, error) {
	const query = `
		SELECT id, user_id, number, balance::float8, currency, created_at
		FROM accounts WHERE id = $1 FOR UPDATE`
	return r.scanOne(ctx, r.q(q), query, id)
}

func (r *AccountRepository) scanOne(ctx context.Context, q Querier, query string, args ...interface{}) (*models.Account, error) {
	a := &models.Account{}
	err := q.QueryRowContext(ctx, query, args...).
		Scan(&a.ID, &a.UserID, &a.Number, &a.Balance, &a.Currency, &a.CreatedAt)
	if err != nil {
		return nil, mapDBError(err)
	}
	return a, nil
}

// ListByUser возвращает все счета пользователя.
func (r *AccountRepository) ListByUser(ctx context.Context, userID int64) ([]models.Account, error) {
	const query = `
		SELECT id, user_id, number, balance::float8, currency, created_at
		FROM accounts WHERE user_id = $1 ORDER BY id`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, mapDBError(err)
	}
	defer rows.Close()

	accounts := make([]models.Account, 0)
	for rows.Next() {
		var a models.Account
		if err := rows.Scan(&a.ID, &a.UserID, &a.Number, &a.Balance, &a.Currency, &a.CreatedAt); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

// AddBalance изменяет баланс на delta (может быть отрицательной).
// Ограничение CHECK (balance >= 0) не даст уйти в минус.
func (r *AccountRepository) AddBalance(ctx context.Context, q Querier, id int64, delta float64) (float64, error) {
	const query = `
		UPDATE accounts SET balance = balance + $2
		WHERE id = $1
		RETURNING balance::float8`
	var balance float64
	err := r.q(q).QueryRowContext(ctx, query, id, delta).Scan(&balance)
	if err != nil {
		return 0, mapDBError(err)
	}
	return balance, nil
}

// TotalBalance возвращает суммарный баланс всех счетов пользователя.
func (r *AccountRepository) TotalBalance(ctx context.Context, userID int64) (float64, int, error) {
	const query = `
		SELECT COALESCE(SUM(balance), 0)::float8, COUNT(*)
		FROM accounts WHERE user_id = $1`
	var total float64
	var count int
	if err := r.db.QueryRowContext(ctx, query, userID).Scan(&total, &count); err != nil {
		return 0, 0, mapDBError(err)
	}
	return total, count, nil
}
