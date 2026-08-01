package service

import (
	"context"
	"database/sql"
	"errors"

	"github.com/sirupsen/logrus"

	"bankapi/internal/models"
	"bankapi/internal/repository"
	"bankapi/internal/security"
)

// CardService реализует выпуск карт, их просмотр и оплату.
type CardService struct {
	db       *sql.DB
	cards    *repository.CardRepository
	accounts *repository.AccountRepository
	txs      *repository.TransactionRepository
	users    *repository.UserRepository
	pgp      *security.PGPCipher
	hmac     *security.HMACSigner
	mailer   *Mailer
	log      *logrus.Logger
}

// NewCardService создаёт сервис карт.
func NewCardService(repos *repository.Repositories, pgp *security.PGPCipher,
	signer *security.HMACSigner, mailer *Mailer, log *logrus.Logger) *CardService {
	return &CardService{
		db:       repos.DB,
		cards:    repos.Card,
		accounts: repos.Account,
		txs:      repos.Transaction,
		users:    repos.User,
		pgp:      pgp,
		hmac:     signer,
		mailer:   mailer,
		log:      log,
	}
}

// Issue выпускает виртуальную карту к счёту пользователя.
// Номер и срок шифруются PGP, CVV хешируется bcrypt, целостность номера
// подтверждается HMAC-SHA256. Открытые данные возвращаются только один раз.
func (s *CardService) Issue(ctx context.Context, userID, accountID int64) (*models.IssuedCard, error) {
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

	number, err := security.GenerateCardNumber()
	if err != nil {
		return nil, err
	}
	cvv, err := security.GenerateCVV()
	if err != nil {
		return nil, err
	}
	expiry := security.GenerateExpiry()

	numberEnc, err := s.pgp.Encrypt(number)
	if err != nil {
		return nil, err
	}
	expiryEnc, err := s.pgp.Encrypt(expiry)
	if err != nil {
		return nil, err
	}
	cvvHash, err := security.HashCVV(cvv)
	if err != nil {
		return nil, err
	}

	card := &models.Card{
		AccountID:       accountID,
		UserID:          userID,
		NumberEncrypted: numberEnc,
		ExpiryEncrypted: expiryEnc,
		NumberHMAC:      s.hmac.Compute(number + "|" + expiry),
		CVVHash:         cvvHash,
		Last4:           number[len(number)-4:],
		Status:          models.StatusActive,
	}
	if err := s.cards.Create(ctx, card); err != nil {
		return nil, err
	}
	s.log.Infof("выпущена карта id=%d к счёту %d", card.ID, accountID)

	if user, uErr := s.users.GetByID(ctx, userID); uErr == nil {
		s.mailer.SendAsync(user.Email, "Выпущена новая карта",
			"<h2>Карта выпущена</h2><p>Карта •••• "+card.Last4+" привязана к счёту №"+acc.Number+".</p>")
	}

	return &models.IssuedCard{
		ID:        card.ID,
		AccountID: accountID,
		Number:    number,
		Expiry:    expiry,
		CVV:       cvv,
		Warning:   "Сохраните данные карты: CVV не хранится в открытом виде и повторно показан не будет.",
	}, nil
}

// decrypt расшифровывает карту и проверяет HMAC-целостность.
func (s *CardService) decrypt(c *models.Card) (number, expiry string, ok bool, err error) {
	number, err = s.pgp.Decrypt(c.NumberEncrypted)
	if err != nil {
		return "", "", false, err
	}
	expiry, err = s.pgp.Decrypt(c.ExpiryEncrypted)
	if err != nil {
		return "", "", false, err
	}
	ok = s.hmac.Verify(number+"|"+expiry, c.NumberHMAC)
	return number, expiry, ok, nil
}

// List возвращает карты пользователя с маскированными номерами.
func (s *CardService) List(ctx context.Context, userID int64) ([]models.CardView, error) {
	cards, err := s.cards.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	views := make([]models.CardView, 0, len(cards))
	for i := range cards {
		number, expiry, ok, err := s.decrypt(&cards[i])
		if err != nil {
			s.log.Errorf("не удалось расшифровать карту id=%d: %v", cards[i].ID, err)
			continue
		}
		views = append(views, models.CardView{
			ID:        cards[i].ID,
			AccountID: cards[i].AccountID,
			Number:    security.MaskCardNumber(number),
			Expiry:    expiry,
			Status:    cards[i].Status,
			Integrity: ok,
			CreatedAt: cards[i].CreatedAt,
		})
	}
	return views, nil
}

// Get возвращает карту владельца. При reveal=true номер отдаётся полностью.
func (s *CardService) Get(ctx context.Context, userID, cardID int64, reveal bool) (*models.CardView, error) {
	card, err := s.cards.GetByID(ctx, cardID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if card.UserID != userID {
		return nil, ErrForbidden
	}
	number, expiry, ok, err := s.decrypt(card)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrIntegrity
	}
	shown := security.MaskCardNumber(number)
	if reveal {
		shown = number
	}
	return &models.CardView{
		ID:        card.ID,
		AccountID: card.AccountID,
		Number:    shown,
		Expiry:    expiry,
		Status:    card.Status,
		Integrity: ok,
		CreatedAt: card.CreatedAt,
	}, nil
}

// Pay проводит оплату картой: проверяет CVV, срок действия и списывает средства.
func (s *CardService) Pay(ctx context.Context, userID, cardID int64, req *models.CardPaymentRequest) (*models.Account, error) {
	card, err := s.cards.GetByID(ctx, cardID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if card.UserID != userID {
		return nil, ErrForbidden
	}
	if card.Status != models.StatusActive {
		return nil, ErrCardBlocked
	}
	if !security.CheckCVV(card.CVVHash, req.CVV) {
		s.log.Warnf("неверный CVV для карты id=%d", cardID)
		return nil, ErrInvalidCVV
	}

	number, expiry, ok, err := s.decrypt(card)
	if err != nil {
		return nil, err
	}
	if !ok {
		s.log.Errorf("нарушена целостность данных карты id=%d", cardID)
		return nil, ErrIntegrity
	}
	if !security.ExpiryValid(expiry) {
		return nil, ErrCardExpired
	}

	amount := round2(req.Amount)
	description := req.Description
	if req.Merchant != "" {
		description = "Оплата в " + req.Merchant + ". " + description
	}

	var account *models.Account
	err = repository.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		acc, err := s.accounts.GetForUpdate(ctx, tx, card.AccountID)
		if err != nil {
			return err
		}
		if acc.Balance < amount {
			return ErrInsufficientFunds
		}
		balance, err := s.accounts.AddBalance(ctx, tx, acc.ID, -amount)
		if err != nil {
			if errors.Is(err, repository.ErrInsufficientFunds) {
				return ErrInsufficientFunds
			}
			return err
		}
		if err := s.txs.Create(ctx, tx, &models.Transaction{
			AccountID:   acc.ID,
			Type:        models.TxCardPayment,
			Amount:      amount,
			Description: description,
		}); err != nil {
			return err
		}
		acc.Balance = balance
		account = acc
		return nil
	})
	if err != nil {
		return nil, err
	}

	s.log.Infof("оплата картой •••• %s на сумму %.2f RUB", card.Last4, amount)
	_ = number // открытый номер за пределы функции не выходит

	if user, uErr := s.users.GetByID(ctx, userID); uErr == nil {
		s.mailer.NotifyPayment(user.Email, "Платёж успешно проведён", amount,
			"Карта •••• "+card.Last4+". "+description)
	}
	return account, nil
}

// SetStatus блокирует или разблокирует карту владельца.
func (s *CardService) SetStatus(ctx context.Context, userID, cardID int64, status string) error {
	card, err := s.cards.GetByID(ctx, cardID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	if card.UserID != userID {
		return ErrForbidden
	}
	return s.cards.UpdateStatus(ctx, cardID, status)
}
