package repository

import (
	"context"
	"database/sql"
	"time"

	"bankapi/internal/models"
)

// TransactionRepository инкапсулирует SQL-запросы к таблице transactions.
type TransactionRepository struct {
	db *sql.DB
}

func (r *TransactionRepository) q(q Querier) Querier {
	if q == nil {
		return r.db
	}
	return q
}

// Create записывает операцию в историю (может вызываться внутри транзакции).
func (r *TransactionRepository) Create(ctx context.Context, q Querier, t *models.Transaction) error {
	const query = `
		INSERT INTO transactions (account_id, counterparty_account_id, type, amount, description)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at`
	err := r.q(q).QueryRowContext(ctx, query,
		t.AccountID, t.CounterpartyID, t.Type, t.Amount, t.Description).
		Scan(&t.ID, &t.CreatedAt)
	return mapDBError(err)
}

// ListByUser возвращает историю операций пользователя за период.
func (r *TransactionRepository) ListByUser(ctx context.Context, userID int64, from, to time.Time, limit int) ([]models.Transaction, error) {
	const query = `
		SELECT t.id, t.account_id, t.counterparty_account_id, t.type,
		       t.amount::float8, t.description, t.created_at
		FROM transactions t
		JOIN accounts a ON a.id = t.account_id
		WHERE a.user_id = $1 AND t.created_at >= $2 AND t.created_at < $3
		ORDER BY t.created_at DESC
		LIMIT $4`
	rows, err := r.db.QueryContext(ctx, query, userID, from, to, limit)
	if err != nil {
		return nil, mapDBError(err)
	}
	defer rows.Close()

	list := make([]models.Transaction, 0)
	for rows.Next() {
		var t models.Transaction
		if err := rows.Scan(&t.ID, &t.AccountID, &t.CounterpartyID, &t.Type,
			&t.Amount, &t.Description, &t.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

// SumByType возвращает суммы операций пользователя в разрезе типа.
func (r *TransactionRepository) SumByType(ctx context.Context, userID int64, from, to time.Time) (map[string]float64, error) {
	const query = `
		SELECT t.type, SUM(t.amount)::float8
		FROM transactions t
		JOIN accounts a ON a.id = t.account_id
		WHERE a.user_id = $1 AND t.created_at >= $2 AND t.created_at < $3
		GROUP BY t.type`
	rows, err := r.db.QueryContext(ctx, query, userID, from, to)
	if err != nil {
		return nil, mapDBError(err)
	}
	defer rows.Close()

	res := make(map[string]float64)
	for rows.Next() {
		var typ string
		var sum float64
		if err := rows.Scan(&typ, &sum); err != nil {
			return nil, err
		}
		res[typ] = sum
	}
	return res, rows.Err()
}

// MonthlyStats агрегирует доходы и расходы по месяцам.
func (r *TransactionRepository) MonthlyStats(ctx context.Context, userID int64, from, to time.Time) ([]models.MonthlyStat, error) {
	const query = `
		SELECT to_char(date_trunc('month', t.created_at), 'YYYY-MM') AS month,
		       COALESCE(SUM(CASE WHEN t.type IN ('deposit','transfer_in','credit_issue')
		                         THEN t.amount ELSE 0 END), 0)::float8 AS income,
		       COALESCE(SUM(CASE WHEN t.type IN ('withdrawal','transfer_out','card_payment','credit_payment')
		                         THEN t.amount ELSE 0 END), 0)::float8 AS expense
		FROM transactions t
		JOIN accounts a ON a.id = t.account_id
		WHERE a.user_id = $1 AND t.created_at >= $2 AND t.created_at < $3
		GROUP BY 1
		ORDER BY 1`
	rows, err := r.db.QueryContext(ctx, query, userID, from, to)
	if err != nil {
		return nil, mapDBError(err)
	}
	defer rows.Close()

	stats := make([]models.MonthlyStat, 0)
	for rows.Next() {
		var s models.MonthlyStat
		if err := rows.Scan(&s.Month, &s.Income, &s.Expense); err != nil {
			return nil, err
		}
		s.Net = s.Income - s.Expense
		stats = append(stats, s)
	}
	return stats, rows.Err()
}
