package repository

import (
	"context"
	"database/sql"
	"time"

	"bankapi/internal/models"
)

// CreditRepository инкапсулирует работу с кредитами и графиками платежей.
type CreditRepository struct {
	db *sql.DB
}

func (r *CreditRepository) q(q Querier) Querier {
	if q == nil {
		return r.db
	}
	return q
}

const creditColumns = `id, user_id, account_id, principal::float8, interest_rate::float8,
	term_months, monthly_payment::float8, total_payment::float8, status, created_at`

// CreateCredit сохраняет кредитный договор.
func (r *CreditRepository) CreateCredit(ctx context.Context, q Querier, c *models.Credit) error {
	const query = `
		INSERT INTO credits (user_id, account_id, principal, interest_rate,
			term_months, monthly_payment, total_payment, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at`
	err := r.q(q).QueryRowContext(ctx, query,
		c.UserID, c.AccountID, c.Principal, c.InterestRate,
		c.TermMonths, c.MonthlyPayment, c.TotalPayment, c.Status).
		Scan(&c.ID, &c.CreatedAt)
	return mapDBError(err)
}

func scanCredit(s interface {
	Scan(dest ...interface{}) error
}) (*models.Credit, error) {
	c := &models.Credit{}
	err := s.Scan(&c.ID, &c.UserID, &c.AccountID, &c.Principal, &c.InterestRate,
		&c.TermMonths, &c.MonthlyPayment, &c.TotalPayment, &c.Status, &c.CreatedAt)
	if err != nil {
		return nil, mapDBError(err)
	}
	return c, nil
}

// GetCredit возвращает кредит по идентификатору.
func (r *CreditRepository) GetCredit(ctx context.Context, id int64) (*models.Credit, error) {
	query := `SELECT ` + creditColumns + ` FROM credits WHERE id = $1`
	return scanCredit(r.db.QueryRowContext(ctx, query, id))
}

// ListCreditsByUser возвращает кредиты пользователя.
func (r *CreditRepository) ListCreditsByUser(ctx context.Context, userID int64) ([]models.Credit, error) {
	query := `SELECT ` + creditColumns + ` FROM credits WHERE user_id = $1 ORDER BY id DESC`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, mapDBError(err)
	}
	defer rows.Close()

	list := make([]models.Credit, 0)
	for rows.Next() {
		c, err := scanCredit(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *c)
	}
	return list, rows.Err()
}

// CreateSchedule массово вставляет строки графика платежей.
func (r *CreditRepository) CreateSchedule(ctx context.Context, q Querier, items []models.PaymentSchedule) error {
	const query = `
		INSERT INTO payment_schedules (credit_id, payment_number, due_date,
			total_amount, principal_amount, interest_amount, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	for _, it := range items {
		if _, err := r.q(q).ExecContext(ctx, query,
			it.CreditID, it.PaymentNumber, it.DueDate,
			it.TotalAmount, it.PrincipalAmount, it.InterestAmount, it.Status); err != nil {
			return mapDBError(err)
		}
	}
	return nil
}

const scheduleColumns = `id, credit_id, payment_number, due_date, total_amount::float8,
	principal_amount::float8, interest_amount::float8, penalty_amount::float8, status, paid_at`

func scanSchedule(s interface {
	Scan(dest ...interface{}) error
}) (*models.PaymentSchedule, error) {
	p := &models.PaymentSchedule{}
	err := s.Scan(&p.ID, &p.CreditID, &p.PaymentNumber, &p.DueDate, &p.TotalAmount,
		&p.PrincipalAmount, &p.InterestAmount, &p.PenaltyAmount, &p.Status, &p.PaidAt)
	if err != nil {
		return nil, mapDBError(err)
	}
	return p, nil
}

// GetSchedule возвращает график платежей по кредиту.
func (r *CreditRepository) GetSchedule(ctx context.Context, creditID int64) ([]models.PaymentSchedule, error) {
	query := `SELECT ` + scheduleColumns + ` FROM payment_schedules
		WHERE credit_id = $1 ORDER BY payment_number`
	rows, err := r.db.QueryContext(ctx, query, creditID)
	if err != nil {
		return nil, mapDBError(err)
	}
	defer rows.Close()

	list := make([]models.PaymentSchedule, 0)
	for rows.Next() {
		p, err := scanSchedule(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *p)
	}
	return list, rows.Err()
}

// DuePayment - строка графика вместе с реквизитами кредита, нужными шедулеру.
type DuePayment struct {
	Schedule  models.PaymentSchedule
	CreditID  int64
	AccountID int64
	UserID    int64
	Email     string
}

// ListDuePayments возвращает неоплаченные платежи со сроком не позднее before.
func (r *CreditRepository) ListDuePayments(ctx context.Context, before time.Time) ([]DuePayment, error) {
	const query = `
		SELECT ps.id, ps.credit_id, ps.payment_number, ps.due_date, ps.total_amount::float8,
		       ps.principal_amount::float8, ps.interest_amount::float8,
		       ps.penalty_amount::float8, ps.status, ps.paid_at,
		       c.account_id, c.user_id, u.email
		FROM payment_schedules ps
		JOIN credits c ON c.id = ps.credit_id
		JOIN users u   ON u.id = c.user_id
		WHERE ps.status IN ('pending','overdue')
		  AND ps.due_date <= $1
		  AND c.status = 'active'
		ORDER BY ps.due_date, ps.id`
	rows, err := r.db.QueryContext(ctx, query, before)
	if err != nil {
		return nil, mapDBError(err)
	}
	defer rows.Close()

	list := make([]DuePayment, 0)
	for rows.Next() {
		var d DuePayment
		p := &d.Schedule
		if err := rows.Scan(&p.ID, &p.CreditID, &p.PaymentNumber, &p.DueDate, &p.TotalAmount,
			&p.PrincipalAmount, &p.InterestAmount, &p.PenaltyAmount, &p.Status, &p.PaidAt,
			&d.AccountID, &d.UserID, &d.Email); err != nil {
			return nil, err
		}
		d.CreditID = p.CreditID
		list = append(list, d)
	}
	return list, rows.Err()
}

// MarkPaid помечает платёж оплаченным.
func (r *CreditRepository) MarkPaid(ctx context.Context, q Querier, scheduleID int64) error {
	const query = `
		UPDATE payment_schedules
		SET status = 'paid', paid_at = now()
		WHERE id = $1 AND status <> 'paid'`
	res, err := r.q(q).ExecContext(ctx, query, scheduleID)
	if err != nil {
		return mapDBError(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ApplyPenalty начисляет штраф за просрочку один раз для платежа.
// Возвращает false, если штраф уже был начислен ранее.
func (r *CreditRepository) ApplyPenalty(ctx context.Context, q Querier, scheduleID int64, rate float64) (bool, error) {
	const query = `
		UPDATE payment_schedules
		SET penalty_amount = ROUND(total_amount * $2, 2), status = 'overdue'
		WHERE id = $1 AND penalty_amount = 0`
	res, err := r.q(q).ExecContext(ctx, query, scheduleID, rate)
	if err != nil {
		return false, mapDBError(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		// Штраф уже есть - фиксируем только статус.
		if _, err := r.q(q).ExecContext(ctx,
			`UPDATE payment_schedules SET status = 'overdue' WHERE id = $1 AND status <> 'paid'`,
			scheduleID); err != nil {
			return false, mapDBError(err)
		}
		return false, nil
	}
	return true, nil
}

// CloseCreditIfPaid переводит кредит в статус closed, если непогашенных платежей нет.
func (r *CreditRepository) CloseCreditIfPaid(ctx context.Context, q Querier, creditID int64) error {
	const query = `
		UPDATE credits SET status = 'closed'
		WHERE id = $1
		  AND status = 'active'
		  AND NOT EXISTS (
		      SELECT 1 FROM payment_schedules
		      WHERE credit_id = $1 AND status <> 'paid'
		  )`
	_, err := r.q(q).ExecContext(ctx, query, creditID)
	return mapDBError(err)
}

// CreditLoad считает агрегированную кредитную нагрузку пользователя.
func (r *CreditRepository) CreditLoad(ctx context.Context, userID int64) (models.CreditLoad, error) {
	var cl models.CreditLoad

	const q1 = `
		SELECT COUNT(*),
		       COALESCE(SUM(principal), 0)::float8,
		       COALESCE(SUM(monthly_payment), 0)::float8
		FROM credits WHERE user_id = $1 AND status = 'active'`
	if err := r.db.QueryRowContext(ctx, q1, userID).
		Scan(&cl.ActiveCredits, &cl.TotalPrincipal, &cl.MonthlyPaymentSum); err != nil {
		return cl, mapDBError(err)
	}

	const q2 = `
		SELECT COALESCE(SUM(ps.total_amount + ps.penalty_amount), 0)::float8,
		       COUNT(*) FILTER (WHERE ps.status = 'overdue'),
		       COALESCE(SUM(ps.total_amount + ps.penalty_amount)
		                FILTER (WHERE ps.status = 'overdue'), 0)::float8
		FROM payment_schedules ps
		JOIN credits c ON c.id = ps.credit_id
		WHERE c.user_id = $1 AND ps.status <> 'paid'`
	if err := r.db.QueryRowContext(ctx, q2, userID).
		Scan(&cl.RemainingDebt, &cl.OverduePayments, &cl.OverdueAmount); err != nil {
		return cl, mapDBError(err)
	}
	return cl, nil
}

// UpcomingPaymentsByAccount возвращает предстоящие платежи по счёту в горизонте прогноза.
func (r *CreditRepository) UpcomingPaymentsByAccount(ctx context.Context, accountID int64, from, to time.Time) ([]models.PaymentSchedule, error) {
	const query = `
		SELECT ps.id, ps.credit_id, ps.payment_number, ps.due_date, ps.total_amount::float8,
		       ps.principal_amount::float8, ps.interest_amount::float8,
		       ps.penalty_amount::float8, ps.status, ps.paid_at
		FROM payment_schedules ps
		JOIN credits c ON c.id = ps.credit_id
		WHERE c.account_id = $1
		  AND ps.status <> 'paid'
		  AND ps.due_date >= $2 AND ps.due_date <= $3
		ORDER BY ps.due_date`
	rows, err := r.db.QueryContext(ctx, query, accountID, from, to)
	if err != nil {
		return nil, mapDBError(err)
	}
	defer rows.Close()

	list := make([]models.PaymentSchedule, 0)
	for rows.Next() {
		p, err := scanSchedule(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *p)
	}
	return list, rows.Err()
}
