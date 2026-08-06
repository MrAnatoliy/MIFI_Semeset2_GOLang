package service

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/sirupsen/logrus"

	"bankapi/internal/models"
	"bankapi/internal/repository"
	"bankapi/internal/security"
)

// AccountService реализует бизнес-логику работы со счетами.
type AccountService struct {
	db       *sql.DB
	accounts *repository.AccountRepository
	txs      *repository.TransactionRepository
	users    *repository.UserRepository
	mailer   *Mailer
	log      *logrus.Logger
}

// NewAccountService создаёт сервис счетов.
func NewAccountService(repos *repository.Repositories, mailer *Mailer, log *logrus.Logger) *AccountService {
	return &AccountService{
		db:       repos.DB,
		accounts: repos.Account,
		txs:      repos.Transaction,
		users:    repos.User,
		mailer:   mailer,
		log:      log,
	}
}

// Create открывает новый рублёвый счёт пользователю.
func (s *AccountService) Create(ctx context.Context, userID int64) (*models.Account, error) {
	// Коллизия номера счёта крайне маловероятна, но обрабатываем её ретраем.
	for attempt := 0; attempt < 5; attempt++ {
		number, err := security.GenerateAccountNumber()
		if err != nil {
			return nil, err
		}
		acc := &models.Account{
			UserID:   userID,
			Number:   number,
			Balance:  0,
			Currency: "RUB",
		}
		err = s.accounts.Create(ctx, acc)
		if err == nil {
			s.log.Infof("открыт счёт id=%d для пользователя %d", acc.ID, userID)
			return acc, nil
		}
		if !errors.Is(err, repository.ErrDuplicate) {
			return nil, err
		}
	}
	return nil, errors.New("не удалось сгенерировать уникальный номер счёта")
}

// List возвращает счета пользователя.
func (s *AccountService) List(ctx context.Context, userID int64) ([]models.Account, error) {
	return s.accounts.ListByUser(ctx, userID)
}

// GetOwned возвращает счёт, проверяя принадлежность пользователю.
func (s *AccountService) GetOwned(ctx context.Context, q repository.Querier, accountID, userID int64) (*models.Account, error) {
	acc, err := s.accounts.GetByID(ctx, q, accountID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if acc.UserID != userID {
		return nil, ErrForbidden
	}
	return acc, nil
}

// Deposit пополняет собственный счёт пользователя.
func (s *AccountService) Deposit(ctx context.Context, userID, accountID int64, amount float64, description string) (*models.Account, error) {
	return s.changeBalance(ctx, userID, accountID, round2(amount), models.TxDeposit, description)
}

// Withdraw списывает средства с собственного счёта пользователя.
func (s *AccountService) Withdraw(ctx context.Context, userID, accountID int64, amount float64, description string) (*models.Account, error) {
	return s.changeBalance(ctx, userID, accountID, -round2(amount), models.TxWithdrawal, description)
}

func (s *AccountService) changeBalance(ctx context.Context, userID, accountID int64,
	delta float64, txType, description string) (*models.Account, error) {

	var result *models.Account
	err := repository.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		acc, err := s.accounts.GetForUpdate(ctx, tx, accountID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return ErrNotFound
			}
			return err
		}
		// Запрет операций с чужим счётом.
		if acc.UserID != userID {
			return ErrForbidden
		}
		if acc.Currency != "RUB" {
			return ErrCurrency
		}
		if delta < 0 && acc.Balance < -delta {
			return ErrInsufficientFunds
		}

		balance, err := s.accounts.AddBalance(ctx, tx, accountID, delta)
		if err != nil {
			if errors.Is(err, repository.ErrInsufficientFunds) {
				return ErrInsufficientFunds
			}
			return err
		}
		amount := delta
		if amount < 0 {
			amount = -amount
		}
		if err := s.txs.Create(ctx, tx, &models.Transaction{
			AccountID:   accountID,
			Type:        txType,
			Amount:      amount,
			Description: description,
		}); err != nil {
			return err
		}
		acc.Balance = balance
		result = acc
		return nil
	})
	if err != nil {
		return nil, err
	}

	if user, uErr := s.users.GetByID(ctx, userID); uErr == nil {
		title := "Пополнение счёта"
		if txType == models.TxWithdrawal {
			title = "Списание со счёта"
		}
		s.mailer.NotifyPayment(user.Email, title, absFloat(delta), "Счёт №"+result.Number)
	}
	return result, nil
}

// Transfer выполняет перевод между счетами в одной транзакции БД.
func (s *AccountService) Transfer(ctx context.Context, userID int64, req *models.TransferRequest) (*models.Account, error) {
	amount := round2(req.Amount)
	var fromAcc *models.Account

	err := repository.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		// Блокируем счета в порядке возрастания ID - защита от взаимоблокировок.
		first, second := req.FromAccountID, req.ToAccountID
		if first > second {
			first, second = second, first
		}
		a1, err := s.accounts.GetForUpdate(ctx, tx, first)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return ErrNotFound
			}
			return err
		}
		a2, err := s.accounts.GetForUpdate(ctx, tx, second)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return ErrNotFound
			}
			return err
		}

		src, dst := a1, a2
		if src.ID != req.FromAccountID {
			src, dst = a2, a1
		}
		// Списывать можно только со своего счёта.
		if src.UserID != userID {
			return ErrForbidden
		}
		if src.Currency != "RUB" || dst.Currency != "RUB" {
			return ErrCurrency
		}
		if src.Balance < amount {
			return ErrInsufficientFunds
		}

		newBalance, err := s.accounts.AddBalance(ctx, tx, src.ID, -amount)
		if err != nil {
			if errors.Is(err, repository.ErrInsufficientFunds) {
				return ErrInsufficientFunds
			}
			return err
		}
		if _, err := s.accounts.AddBalance(ctx, tx, dst.ID, amount); err != nil {
			return err
		}

		if err := s.txs.Create(ctx, tx, &models.Transaction{
			AccountID:      src.ID,
			CounterpartyID: &dst.ID,
			Type:           models.TxTransferOut,
			Amount:         amount,
			Description:    req.Description,
		}); err != nil {
			return err
		}
		if err := s.txs.Create(ctx, tx, &models.Transaction{
			AccountID:      dst.ID,
			CounterpartyID: &src.ID,
			Type:           models.TxTransferIn,
			Amount:         amount,
			Description:    req.Description,
		}); err != nil {
			return err
		}

		src.Balance = newBalance
		fromAcc = src
		return nil
	})
	if err != nil {
		return nil, err
	}

	s.log.Infof("перевод %.2f RUB со счёта %d на счёт %d", amount, req.FromAccountID, req.ToAccountID)
	if user, uErr := s.users.GetByID(ctx, userID); uErr == nil {
		s.mailer.NotifyPayment(user.Email, "Перевод выполнен", amount,
			"Со счёта №"+fromAcc.Number)
	}
	return fromAcc, nil
}

// History возвращает историю операций пользователя.
func (s *AccountService) History(ctx context.Context, userID int64, limit int) ([]models.Transaction, error) {
	from := time.Now().AddDate(-100, 0, 0)
	to := time.Now().Add(24 * time.Hour)
	return s.txs.ListByUser(ctx, userID, from, to, limit)
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
