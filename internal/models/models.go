package models

import "time"

// Типы транзакций.
const (
	TxDeposit       = "deposit"
	TxWithdrawal    = "withdrawal"
	TxTransferIn    = "transfer_in"
	TxTransferOut   = "transfer_out"
	TxCardPayment   = "card_payment"
	TxCreditIssue   = "credit_issue"
	TxCreditPayment = "credit_payment"
)

// Статусы.
const (
	StatusActive    = "active"
	StatusBlocked   = "blocked"
	StatusClosed    = "closed"
	StatusPending   = "pending"
	StatusPaid      = "paid"
	StatusOverdue   = "overdue"
	StatusDefaulted = "defaulted"
)

// User — пользователь сервиса.
type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

// Account — банковский счёт.
type Account struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Number    string    `json:"number"`
	Balance   float64   `json:"balance"`
	Currency  string    `json:"currency"`
	CreatedAt time.Time `json:"created_at"`
}

// Card — банковская карта. Чувствительные поля хранятся в зашифрованном виде
// и никогда не сериализуются в JSON.
type Card struct {
	ID              int64     `json:"id"`
	AccountID       int64     `json:"account_id"`
	UserID          int64     `json:"user_id"`
	NumberEncrypted []byte    `json:"-"`
	ExpiryEncrypted []byte    `json:"-"`
	NumberHMAC      string    `json:"-"`
	CVVHash         string    `json:"-"`
	Last4           string    `json:"last4"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
}

// CardView — представление карты для владельца (после расшифровки).
type CardView struct {
	ID        int64     `json:"id"`
	AccountID int64     `json:"account_id"`
	Number    string    `json:"number"` // маскированный или полный
	Expiry    string    `json:"expiry"`
	Status    string    `json:"status"`
	Integrity bool      `json:"integrity_ok"`
	CreatedAt time.Time `json:"created_at"`
}

// IssuedCard возвращается один раз в момент выпуска карты.
type IssuedCard struct {
	ID        int64  `json:"id"`
	AccountID int64  `json:"account_id"`
	Number    string `json:"number"`
	Expiry    string `json:"expiry"`
	CVV       string `json:"cvv"`
	Warning   string `json:"warning"`
}

// Transaction — операция по счёту.
type Transaction struct {
	ID             int64     `json:"id"`
	AccountID      int64     `json:"account_id"`
	CounterpartyID *int64    `json:"counterparty_account_id,omitempty"`
	Type           string    `json:"type"`
	Amount         float64   `json:"amount"`
	Description    string    `json:"description"`
	CreatedAt      time.Time `json:"created_at"`
}

// Credit — кредитный договор.
type Credit struct {
	ID             int64     `json:"id"`
	UserID         int64     `json:"user_id"`
	AccountID      int64     `json:"account_id"`
	Principal      float64   `json:"principal"`
	InterestRate   float64   `json:"interest_rate"`
	TermMonths     int       `json:"term_months"`
	MonthlyPayment float64   `json:"monthly_payment"`
	TotalPayment   float64   `json:"total_payment"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
}

// PaymentSchedule — строка графика платежей.
type PaymentSchedule struct {
	ID              int64      `json:"id"`
	CreditID        int64      `json:"credit_id"`
	PaymentNumber   int        `json:"payment_number"`
	DueDate         time.Time  `json:"due_date"`
	TotalAmount     float64    `json:"total_amount"`
	PrincipalAmount float64    `json:"principal_amount"`
	InterestAmount  float64    `json:"interest_amount"`
	PenaltyAmount   float64    `json:"penalty_amount"`
	Status          string     `json:"status"`
	PaidAt          *time.Time `json:"paid_at,omitempty"`
}

// Payable — сумма к списанию с учётом штрафа.
func (p PaymentSchedule) Payable() float64 {
	return p.TotalAmount + p.PenaltyAmount
}
