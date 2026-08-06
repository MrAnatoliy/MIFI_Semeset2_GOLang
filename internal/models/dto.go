package models

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
)

var (
	emailRe    = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	usernameRe = regexp.MustCompile(`^[a-zA-Z0-9_.\-]{3,64}$`)
)

// ErrValidation - базовая ошибка валидации входных данных.
var ErrValidation = errors.New("validation error")

// RegisterRequest - тело запроса POST /register.
type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (r *RegisterRequest) Validate() error {
	r.Username = strings.TrimSpace(r.Username)
	r.Email = strings.ToLower(strings.TrimSpace(r.Email))

	if !usernameRe.MatchString(r.Username) {
		return errors.New("username: 3-64 символа, допустимы буквы, цифры, _ . -")
	}
	if !emailRe.MatchString(r.Email) || len(r.Email) > 255 {
		return errors.New("email: некорректный формат")
	}
	return validatePassword(r.Password)
}

func validatePassword(p string) error {
	if len(p) < 8 || len(p) > 72 { // 72 - предел bcrypt
		return errors.New("password: от 8 до 72 символов")
	}
	var hasDigit, hasLetter bool
	for _, c := range p {
		switch {
		case unicode.IsDigit(c):
			hasDigit = true
		case unicode.IsLetter(c):
			hasLetter = true
		}
	}
	if !hasDigit || !hasLetter {
		return errors.New("password: должен содержать буквы и цифры")
	}
	return nil
}

// LoginRequest - тело запроса POST /login. Логин - email или username.
type LoginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

func (r *LoginRequest) Validate() error {
	r.Login = strings.TrimSpace(r.Login)
	if r.Login == "" || r.Password == "" {
		return errors.New("login и password обязательны")
	}
	return nil
}

// AuthResponse - ответ на регистрацию/логин.
type AuthResponse struct {
	Token     string `json:"token"`
	ExpiresIn int64  `json:"expires_in"`
	User      *User  `json:"user"`
}

// AmountRequest - операции пополнения/списания.
type AmountRequest struct {
	Amount      float64 `json:"amount"`
	Description string  `json:"description"`
}

func (r *AmountRequest) Validate() error {
	if r.Amount <= 0 {
		return errors.New("amount: должно быть больше нуля")
	}
	if r.Amount > 1_000_000_000 {
		return errors.New("amount: превышен допустимый лимит операции")
	}
	return nil
}

// TransferRequest - тело запроса POST /transfer.
type TransferRequest struct {
	FromAccountID int64   `json:"from_account_id"`
	ToAccountID   int64   `json:"to_account_id"`
	Amount        float64 `json:"amount"`
	Description   string  `json:"description"`
}

func (r *TransferRequest) Validate() error {
	if r.FromAccountID <= 0 || r.ToAccountID <= 0 {
		return errors.New("from_account_id и to_account_id обязательны")
	}
	if r.FromAccountID == r.ToAccountID {
		return errors.New("нельзя переводить средства на тот же счёт")
	}
	if r.Amount <= 0 {
		return errors.New("amount: должно быть больше нуля")
	}
	return nil
}

// CreateCardRequest - тело запроса POST /cards.
type CreateCardRequest struct {
	AccountID int64 `json:"account_id"`
}

func (r *CreateCardRequest) Validate() error {
	if r.AccountID <= 0 {
		return errors.New("account_id обязателен")
	}
	return nil
}

// CardPaymentRequest - тело запроса POST /cards/{cardId}/pay.
type CardPaymentRequest struct {
	CVV         string  `json:"cvv"`
	Amount      float64 `json:"amount"`
	Merchant    string  `json:"merchant"`
	Description string  `json:"description"`
}

func (r *CardPaymentRequest) Validate() error {
	if len(r.CVV) != 3 {
		return errors.New("cvv: ожидается 3 цифры")
	}
	if r.Amount <= 0 {
		return errors.New("amount: должно быть больше нуля")
	}
	return nil
}

// CreateCreditRequest - тело запроса POST /credits.
type CreateCreditRequest struct {
	AccountID  int64   `json:"account_id"`
	Amount     float64 `json:"amount"`
	TermMonths int     `json:"term_months"`
}

func (r *CreateCreditRequest) Validate() error {
	if r.AccountID <= 0 {
		return errors.New("account_id обязателен")
	}
	if r.Amount < 1000 {
		return errors.New("amount: минимальная сумма кредита 1000 RUB")
	}
	if r.Amount > 100_000_000 {
		return errors.New("amount: превышен лимит кредитования")
	}
	if r.TermMonths < 1 || r.TermMonths > 360 {
		return errors.New("term_months: от 1 до 360")
	}
	return nil
}

// MonthlyStat - статистика за календарный месяц.
type MonthlyStat struct {
	Month   string  `json:"month"`
	Income  float64 `json:"income"`
	Expense float64 `json:"expense"`
	Net     float64 `json:"net"`
}

// CreditLoad - аналитика кредитной нагрузки.
type CreditLoad struct {
	ActiveCredits        int     `json:"active_credits"`
	TotalPrincipal       float64 `json:"total_principal"`
	MonthlyPaymentSum    float64 `json:"monthly_payment_sum"`
	RemainingDebt        float64 `json:"remaining_debt"`
	OverduePayments      int     `json:"overdue_payments"`
	OverdueAmount        float64 `json:"overdue_amount"`
	AvgMonthlyIncome     float64 `json:"avg_monthly_income"`
	DebtToIncomeRatioPct float64 `json:"debt_to_income_ratio_pct"`
}

// AnalyticsResponse - агрегированный ответ GET /analytics.
type AnalyticsResponse struct {
	From          string             `json:"from"`
	To            string             `json:"to"`
	TotalIncome   float64            `json:"total_income"`
	TotalExpense  float64            `json:"total_expense"`
	Net           float64            `json:"net"`
	ByType        map[string]float64 `json:"by_type"`
	Monthly       []MonthlyStat      `json:"monthly"`
	CreditLoad    CreditLoad         `json:"credit_load"`
	TotalBalance  float64            `json:"total_balance"`
	AccountsCount int                `json:"accounts_count"`
}

// PredictionDay - прогноз на конкретный день.
type PredictionDay struct {
	Date             string  `json:"date"`
	ScheduledOutflow float64 `json:"scheduled_outflow"`
	Balance          float64 `json:"balance"`
}

// PredictionResponse - ответ GET /accounts/{accountId}/predict.
type PredictionResponse struct {
	AccountID      int64           `json:"account_id"`
	Days           int             `json:"days"`
	CurrentBalance float64         `json:"current_balance"`
	TotalScheduled float64         `json:"total_scheduled_payments"`
	FinalBalance   float64         `json:"final_balance"`
	Negative       bool            `json:"will_go_negative"`
	Timeline       []PredictionDay `json:"timeline"`
}
