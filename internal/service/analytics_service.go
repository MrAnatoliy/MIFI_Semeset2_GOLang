package service

import (
	"context"
	"errors"
	"time"

	"github.com/sirupsen/logrus"

	"bankapi/internal/config"
	"bankapi/internal/models"
	"bankapi/internal/repository"
)

// AnalyticsService предоставляет клиенту финансовую аналитику.
type AnalyticsService struct {
	accounts *repository.AccountRepository
	txs      *repository.TransactionRepository
	credits  *repository.CreditRepository
	cfg      *config.Config
	log      *logrus.Logger
}

// NewAnalyticsService создаёт сервис аналитики.
func NewAnalyticsService(repos *repository.Repositories, cfg *config.Config, log *logrus.Logger) *AnalyticsService {
	return &AnalyticsService{
		accounts: repos.Account,
		txs:      repos.Transaction,
		credits:  repos.Credit,
		cfg:      cfg,
		log:      log,
	}
}

var incomeTypes = map[string]bool{
	models.TxDeposit:     true,
	models.TxTransferIn:  true,
	models.TxCreditIssue: true,
}

// Overview собирает статистику доходов/расходов и кредитной нагрузки за период.
// По умолчанию период - последние 12 месяцев.
func (s *AnalyticsService) Overview(ctx context.Context, userID int64, from, to time.Time) (*models.AnalyticsResponse, error) {
	byType, err := s.txs.SumByType(ctx, userID, from, to)
	if err != nil {
		return nil, err
	}
	monthly, err := s.txs.MonthlyStats(ctx, userID, from, to)
	if err != nil {
		return nil, err
	}
	load, err := s.credits.CreditLoad(ctx, userID)
	if err != nil {
		return nil, err
	}
	totalBalance, accCount, err := s.accounts.TotalBalance(ctx, userID)
	if err != nil {
		return nil, err
	}

	var income, expense float64
	for typ, sum := range byType {
		if incomeTypes[typ] {
			income += sum
		} else {
			expense += sum
		}
	}

	// Средний ежемесячный доход и показатель долговой нагрузки (ПДН).
	if len(monthly) > 0 {
		var sum float64
		for _, m := range monthly {
			sum += m.Income
		}
		load.AvgMonthlyIncome = round2(sum / float64(len(monthly)))
	}
	if load.AvgMonthlyIncome > 0 {
		load.DebtToIncomeRatioPct = round2(load.MonthlyPaymentSum / load.AvgMonthlyIncome * 100)
	}

	return &models.AnalyticsResponse{
		From:          from.Format("2006-01-02"),
		To:            to.Format("2006-01-02"),
		TotalIncome:   round2(income),
		TotalExpense:  round2(expense),
		Net:           round2(income - expense),
		ByType:        byType,
		Monthly:       monthly,
		CreditLoad:    load,
		TotalBalance:  round2(totalBalance),
		AccountsCount: accCount,
	}, nil
}

// Predict строит прогноз баланса счёта на N дней с учётом запланированных
// платежей по кредитам. Максимальный горизонт - 365 дней.
func (s *AnalyticsService) Predict(ctx context.Context, userID, accountID int64, days int) (*models.PredictionResponse, error) {
	if days < 1 || days > s.cfg.MaxPredictDays {
		return nil, ErrPredictRange
	}
	acc, err := s.accounts.GetByID(ctx, nil, accountID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if acc.UserID != userID {
		return nil, ErrForbidden
	}

	today := time.Now().Truncate(24 * time.Hour)
	end := today.AddDate(0, 0, days)

	payments, err := s.credits.UpcomingPaymentsByAccount(ctx, accountID, today, end)
	if err != nil {
		return nil, err
	}

	// Группируем платежи по дате.
	outflow := make(map[string]float64)
	var total float64
	for _, p := range payments {
		key := p.DueDate.Format("2006-01-02")
		outflow[key] += p.Payable()
		total += p.Payable()
	}

	balance := acc.Balance
	negative := false
	timeline := make([]models.PredictionDay, 0, days)
	for d := 1; d <= days; d++ {
		day := today.AddDate(0, 0, d)
		key := day.Format("2006-01-02")
		balance = round2(balance - outflow[key])
		if balance < 0 {
			negative = true
		}
		timeline = append(timeline, models.PredictionDay{
			Date:             key,
			ScheduledOutflow: round2(outflow[key]),
			Balance:          balance,
		})
	}

	return &models.PredictionResponse{
		AccountID:      accountID,
		Days:           days,
		CurrentBalance: round2(acc.Balance),
		TotalScheduled: round2(total),
		FinalBalance:   balance,
		Negative:       negative,
		Timeline:       timeline,
	}, nil
}
