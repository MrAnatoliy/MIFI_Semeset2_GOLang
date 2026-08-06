package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/sirupsen/logrus"

	"bankapi/internal/config"
	"bankapi/internal/models"
	"bankapi/internal/repository"
)

// CreditService реализует оформление кредитов, график платежей
// и автоматическое списание с начислением штрафов за просрочку.
type CreditService struct {
	db       *sql.DB
	credits  *repository.CreditRepository
	accounts *repository.AccountRepository
	txs      *repository.TransactionRepository
	users    *repository.UserRepository
	cbr      *CBRService
	mailer   *Mailer
	cfg      *config.Config
	log      *logrus.Logger
}

// NewCreditService создаёт кредитный сервис.
func NewCreditService(repos *repository.Repositories, cbr *CBRService, mailer *Mailer,
	cfg *config.Config, log *logrus.Logger) *CreditService {
	return &CreditService{
		db:       repos.DB,
		credits:  repos.Credit,
		accounts: repos.Account,
		txs:      repos.Transaction,
		users:    repos.User,
		cbr:      cbr,
		mailer:   mailer,
		cfg:      cfg,
		log:      log,
	}
}

// AnnuityPayment рассчитывает ежемесячный аннуитетный платёж:
//
//	P = S * i * (1+i)^n / ((1+i)^n - 1),
//
// где S - сумма кредита, i - месячная ставка, n - срок в месяцах.
func AnnuityPayment(principal, annualRatePct float64, months int) float64 {
	if months <= 0 {
		return 0
	}
	i := annualRatePct / 100 / 12
	if i <= 0 {
		return round2(principal / float64(months))
	}
	pow := math.Pow(1+i, float64(months))
	return round2(principal * i * pow / (pow - 1))
}

// BuildSchedule формирует график платежей с разбивкой на проценты и тело долга.
func BuildSchedule(creditID int64, principal, annualRatePct float64, months int, start time.Time) []models.PaymentSchedule {
	monthly := AnnuityPayment(principal, annualRatePct, months)
	i := annualRatePct / 100 / 12
	remaining := principal

	schedule := make([]models.PaymentSchedule, 0, months)
	for n := 1; n <= months; n++ {
		interest := round2(remaining * i)
		principalPart := round2(monthly - interest)
		total := monthly

		if n == months {
			// Последний платёж закрывает остаток без накопленной погрешности округления.
			principalPart = round2(remaining)
			total = round2(principalPart + interest)
		}
		remaining = round2(remaining - principalPart)
		if remaining < 0 {
			remaining = 0
		}

		schedule = append(schedule, models.PaymentSchedule{
			CreditID:        creditID,
			PaymentNumber:   n,
			DueDate:         start.AddDate(0, n, 0),
			TotalAmount:     total,
			PrincipalAmount: principalPart,
			InterestAmount:  interest,
			Status:          models.StatusPending,
		})
	}
	return schedule
}

// CreditWithSchedule - кредит вместе с графиком платежей.
type CreditWithSchedule struct {
	Credit   *models.Credit           `json:"credit"`
	Schedule []models.PaymentSchedule `json:"schedule"`
}

// Create оформляет кредит: ставка = ключевая ставка ЦБ РФ + маржа банка,
// сумма зачисляется на счёт заёмщика, формируется график платежей.
func (s *CreditService) Create(ctx context.Context, userID int64, req *models.CreateCreditRequest) (*CreditWithSchedule, error) {
	acc, err := s.accounts.GetByID(ctx, nil, req.AccountID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if acc.UserID != userID {
		return nil, ErrForbidden
	}
	if acc.Currency != "RUB" {
		return nil, ErrCurrency
	}

	rate := s.cbr.CreditRate(ctx)
	if rate < s.cfg.MinCreditRate {
		rate = s.cfg.MinCreditRate
	}
	if rate > s.cfg.MaxCreditRate {
		rate = s.cfg.MaxCreditRate
	}

	principal := round2(req.Amount)
	monthly := AnnuityPayment(principal, rate, req.TermMonths)

	credit := &models.Credit{
		UserID:         userID,
		AccountID:      acc.ID,
		Principal:      principal,
		InterestRate:   rate,
		TermMonths:     req.TermMonths,
		MonthlyPayment: monthly,
		TotalPayment:   round2(monthly * float64(req.TermMonths)),
		Status:         models.StatusActive,
	}

	var schedule []models.PaymentSchedule
	err = repository.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		if err := s.credits.CreateCredit(ctx, tx, credit); err != nil {
			return err
		}
		schedule = BuildSchedule(credit.ID, principal, rate, req.TermMonths, time.Now())
		if err := s.credits.CreateSchedule(ctx, tx, schedule); err != nil {
			return err
		}
		if _, err := s.accounts.AddBalance(ctx, tx, acc.ID, principal); err != nil {
			return err
		}
		return s.txs.Create(ctx, tx, &models.Transaction{
			AccountID:   acc.ID,
			Type:        models.TxCreditIssue,
			Amount:      principal,
			Description: fmt.Sprintf("Выдача кредита №%d под %.2f%% годовых", credit.ID, rate),
		})
	})
	if err != nil {
		return nil, err
	}

	s.log.Infof("оформлен кредит id=%d на %.2f RUB под %.2f%% на %d мес.",
		credit.ID, principal, rate, req.TermMonths)

	if user, uErr := s.users.GetByID(ctx, userID); uErr == nil {
		s.mailer.NotifyPayment(user.Email,
			fmt.Sprintf("Кредит №%d оформлен", credit.ID), principal,
			fmt.Sprintf("Ставка %.2f%% годовых, ежемесячный платёж %.2f RUB.", rate, monthly))
	}

	return &CreditWithSchedule{Credit: credit, Schedule: schedule}, nil
}

// List возвращает кредиты пользователя.
func (s *CreditService) List(ctx context.Context, userID int64) ([]models.Credit, error) {
	return s.credits.ListCreditsByUser(ctx, userID)
}

// Schedule возвращает график платежей по кредиту владельца.
func (s *CreditService) Schedule(ctx context.Context, userID, creditID int64) (*CreditWithSchedule, error) {
	credit, err := s.credits.GetCredit(ctx, creditID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if credit.UserID != userID {
		return nil, ErrForbidden
	}
	schedule, err := s.credits.GetSchedule(ctx, creditID)
	if err != nil {
		return nil, err
	}
	return &CreditWithSchedule{Credit: credit, Schedule: schedule}, nil
}

// ProcessDuePayments - задача шедулера: списывает наступившие платежи,
// а при нехватке средств начисляет штраф +10% (единоразово) и отмечает просрочку.
func (s *CreditService) ProcessDuePayments(ctx context.Context) {
	due, err := s.credits.ListDuePayments(ctx, time.Now())
	if err != nil {
		s.log.Errorf("шедулер: не удалось получить список платежей: %v", err)
		return
	}
	if len(due) == 0 {
		s.log.Debug("шедулер: платежей к списанию нет")
		return
	}
	s.log.Infof("шедулер: к обработке %d платеж(ей)", len(due))

	for _, item := range due {
		s.processOne(ctx, item)
	}
}

func (s *CreditService) processOne(ctx context.Context, item repository.DuePayment) {
	amount := round2(item.Schedule.Payable())

	err := repository.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		acc, err := s.accounts.GetForUpdate(ctx, tx, item.AccountID)
		if err != nil {
			return err
		}
		if acc.Balance < amount {
			return ErrInsufficientFunds
		}
		if _, err := s.accounts.AddBalance(ctx, tx, acc.ID, -amount); err != nil {
			return err
		}
		if err := s.txs.Create(ctx, tx, &models.Transaction{
			AccountID: acc.ID,
			Type:      models.TxCreditPayment,
			Amount:    amount,
			Description: fmt.Sprintf("Платёж №%d по кредиту №%d",
				item.Schedule.PaymentNumber, item.CreditID),
		}); err != nil {
			return err
		}
		if err := s.credits.MarkPaid(ctx, tx, item.Schedule.ID); err != nil {
			return err
		}
		return s.credits.CloseCreditIfPaid(ctx, tx, item.CreditID)
	})

	if err == nil {
		s.log.Infof("шедулер: списан платёж №%d по кредиту №%d на %.2f RUB",
			item.Schedule.PaymentNumber, item.CreditID, amount)
		s.mailer.NotifyPayment(item.Email,
			fmt.Sprintf("Платёж по кредиту №%d проведён", item.CreditID), amount,
			fmt.Sprintf("Платёж №%d по графику.", item.Schedule.PaymentNumber))
		return
	}

	if !errors.Is(err, ErrInsufficientFunds) {
		s.log.Errorf("шедулер: ошибка списания платежа id=%d: %v", item.Schedule.ID, err)
		return
	}

	// Недостаточно средств - фиксируем просрочку и начисляем штраф один раз.
	applied, pErr := s.credits.ApplyPenalty(ctx, nil, item.Schedule.ID, s.cfg.PenaltyRate)
	if pErr != nil {
		s.log.Errorf("шедулер: не удалось начислить штраф по платежу id=%d: %v", item.Schedule.ID, pErr)
		return
	}
	if applied {
		penalty := round2(item.Schedule.TotalAmount * s.cfg.PenaltyRate)
		s.log.Warnf("шедулер: просрочка платежа №%d по кредиту №%d, начислен штраф %.2f RUB",
			item.Schedule.PaymentNumber, item.CreditID, penalty)
		s.mailer.NotifyOverdue(item.Email, item.CreditID, item.Schedule.TotalAmount, penalty)
	} else {
		s.log.Warnf("шедулер: платёж №%d по кредиту №%d по-прежнему не оплачен",
			item.Schedule.PaymentNumber, item.CreditID)
	}
}
