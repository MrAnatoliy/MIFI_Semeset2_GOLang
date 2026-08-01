package repository

import (
	"context"
	"database/sql"

	"bankapi/internal/models"
)

// CardRepository инкапсулирует SQL-запросы к таблице cards.
type CardRepository struct {
	db *sql.DB
}

const cardColumns = `id, account_id, user_id, number_encrypted, expiry_encrypted,
	number_hmac, cvv_hash, last4, status, created_at`

// Create сохраняет выпущенную карту (все чувствительные поля уже защищены).
func (r *CardRepository) Create(ctx context.Context, c *models.Card) error {
	const query = `
		INSERT INTO cards (account_id, user_id, number_encrypted, expiry_encrypted,
			number_hmac, cvv_hash, last4, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at`
	err := r.db.QueryRowContext(ctx, query,
		c.AccountID, c.UserID, c.NumberEncrypted, c.ExpiryEncrypted,
		c.NumberHMAC, c.CVVHash, c.Last4, c.Status).
		Scan(&c.ID, &c.CreatedAt)
	return mapDBError(err)
}

func scanCard(s interface {
	Scan(dest ...interface{}) error
}) (*models.Card, error) {
	c := &models.Card{}
	err := s.Scan(&c.ID, &c.AccountID, &c.UserID, &c.NumberEncrypted, &c.ExpiryEncrypted,
		&c.NumberHMAC, &c.CVVHash, &c.Last4, &c.Status, &c.CreatedAt)
	if err != nil {
		return nil, mapDBError(err)
	}
	return c, nil
}

// GetByID возвращает карту по идентификатору.
func (r *CardRepository) GetByID(ctx context.Context, id int64) (*models.Card, error) {
	query := `SELECT ` + cardColumns + ` FROM cards WHERE id = $1`
	return scanCard(r.db.QueryRowContext(ctx, query, id))
}

// ListByUser возвращает все карты пользователя.
func (r *CardRepository) ListByUser(ctx context.Context, userID int64) ([]models.Card, error) {
	query := `SELECT ` + cardColumns + ` FROM cards WHERE user_id = $1 ORDER BY id`
	return r.list(ctx, query, userID)
}

// ListByAccount возвращает карты, привязанные к счёту.
func (r *CardRepository) ListByAccount(ctx context.Context, accountID int64) ([]models.Card, error) {
	query := `SELECT ` + cardColumns + ` FROM cards WHERE account_id = $1 ORDER BY id`
	return r.list(ctx, query, accountID)
}

func (r *CardRepository) list(ctx context.Context, query string, args ...interface{}) ([]models.Card, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, mapDBError(err)
	}
	defer rows.Close()

	cards := make([]models.Card, 0)
	for rows.Next() {
		c, err := scanCard(rows)
		if err != nil {
			return nil, err
		}
		cards = append(cards, *c)
	}
	return cards, rows.Err()
}

// UpdateStatus меняет статус карты (active/blocked).
func (r *CardRepository) UpdateStatus(ctx context.Context, id int64, status string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE cards SET status = $2 WHERE id = $1`, id, status)
	if err != nil {
		return mapDBError(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
