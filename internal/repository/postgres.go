package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/sirupsen/logrus"

	"bankapi/internal/config"
)

// Доменные ошибки уровня хранилища.
var (
	ErrNotFound          = errors.New("запись не найдена")
	ErrDuplicate         = errors.New("запись уже существует")
	ErrInsufficientFunds = errors.New("недостаточно средств на счёте")
)

// Querier абстрагирует *sql.DB и *sql.Tx, что позволяет переиспользовать
// методы репозиториев как вне, так и внутри транзакции.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

// NewPostgres открывает пул соединений и дожидается готовности БД.
func NewPostgres(cfg *config.Config, log *logrus.Logger) (*sql.DB, error) {
	db, err := sql.Open("postgres", cfg.DB.DSN())
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(time.Hour)

	var lastErr error
	for attempt := 1; attempt <= 15; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		lastErr = db.PingContext(ctx)
		cancel()
		if lastErr == nil {
			log.Info("подключение к PostgreSQL установлено")
			return db, nil
		}
		log.Warnf("БД недоступна (попытка %d/15): %v", attempt, lastErr)
		time.Sleep(2 * time.Second)
	}
	_ = db.Close()
	return nil, fmt.Errorf("ping postgres: %w", lastErr)
}

// WithTx выполняет fn в транзакции с корректным откатом при ошибке или панике.
func WithTx(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// RunMigrations последовательно применяет .sql-файлы из каталога миграций.
func RunMigrations(ctx context.Context, db *sql.DB, dir string, log *logrus.Logger) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read migrations dir %q: %w", dir, err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, name := range files {
		var exists bool
		if err := db.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`, name).
			Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		err = WithTx(ctx, db, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, string(body)); err != nil {
				return fmt.Errorf("migration %s: %w", name, err)
			}
			_, err := tx.ExecContext(ctx,
				`INSERT INTO schema_migrations(version) VALUES ($1)`, name)
			return err
		})
		if err != nil {
			return err
		}
		log.Infof("миграция применена: %s", name)
	}
	return nil
}

// mapDBError переводит ошибки драйвера в доменные.
func mapDBError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		switch pqErr.Code {
		case "23505": // unique_violation
			return ErrDuplicate
		case "23514": // check_violation — сработал CHECK (balance >= 0)
			return ErrInsufficientFunds
		}
	}
	return err
}

// Repositories агрегирует все репозитории приложения.
type Repositories struct {
	DB          *sql.DB
	User        *UserRepository
	Account     *AccountRepository
	Card        *CardRepository
	Transaction *TransactionRepository
	Credit      *CreditRepository
}

// New собирает набор репозиториев поверх одного пула соединений.
func New(db *sql.DB) *Repositories {
	return &Repositories{
		DB:          db,
		User:        &UserRepository{db: db},
		Account:     &AccountRepository{db: db},
		Card:        &CardRepository{db: db},
		Transaction: &TransactionRepository{db: db},
		Credit:      &CreditRepository{db: db},
	}
}
