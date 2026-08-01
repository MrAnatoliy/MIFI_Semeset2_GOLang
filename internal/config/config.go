package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// DBConfig — параметры подключения к PostgreSQL.
type DBConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
}

// DSN формирует строку подключения для lib/pq.
func (d DBConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.Name, d.SSLMode,
	)
}

// SMTPConfig — параметры почтового сервера.
type SMTPConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	From     string
	Enabled  bool
}

// Config — конфигурация всего приложения.
type Config struct {
	AppPort       string
	LogLevel      string
	MigrationsDir string

	DB   DBConfig
	SMTP SMTPConfig

	JWTSecret     string
	JWTTTL        time.Duration
	HMACSecret    string
	PGPPassphrase string

	SchedulerInterval time.Duration
	BankMargin        float64 // маржа банка поверх ключевой ставки ЦБ, п.п.
	FallbackRate      float64 // ставка, если ЦБ РФ недоступен
	MinCreditRate     float64
	MaxCreditRate     float64
	PenaltyRate       float64 // штраф за просрочку (0.10 = +10%)

	MaxPredictDays int
	Currency       string
}

// Load читает конфигурацию из переменных окружения, подставляя значения по умолчанию.
func Load() *Config {
	cfg := &Config{
		AppPort:       getEnv("APP_PORT", "8080"),
		LogLevel:      getEnv("LOG_LEVEL", "info"),
		MigrationsDir: getEnv("MIGRATIONS_DIR", "./migrations"),

		DB: DBConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "bank"),
			Password: getEnv("DB_PASSWORD", "bank"),
			Name:     getEnv("DB_NAME", "bank"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},

		SMTP: SMTPConfig{
			Host:     getEnv("SMTP_HOST", "localhost"),
			Port:     getEnvInt("SMTP_PORT", 1025),
			User:     getEnv("SMTP_USER", ""),
			Password: getEnv("SMTP_PASSWORD", ""),
			From:     getEnv("SMTP_FROM", "noreply@bank.local"),
			Enabled:  getEnvBool("SMTP_ENABLED", true),
		},

		JWTSecret:     getEnv("JWT_SECRET", "change-me-in-production"),
		JWTTTL:        time.Duration(getEnvInt("JWT_TTL_HOURS", 24)) * time.Hour,
		HMACSecret:    getEnv("HMAC_SECRET", "change-me-hmac-secret"),
		PGPPassphrase: getEnv("PGP_PASSPHRASE", "change-me-pgp-passphrase"),

		SchedulerInterval: time.Duration(getEnvInt("SCHEDULER_INTERVAL_HOURS", 12)) * time.Hour,
		BankMargin:        getEnvFloat("BANK_MARGIN", 5),
		FallbackRate:      getEnvFloat("FALLBACK_KEY_RATE", 16),
		MinCreditRate:     getEnvFloat("MIN_CREDIT_RATE", 1),
		MaxCreditRate:     getEnvFloat("MAX_CREDIT_RATE", 100),
		PenaltyRate:       getEnvFloat("PENALTY_RATE", 0.10),

		MaxPredictDays: getEnvInt("MAX_PREDICT_DAYS", 365),
		Currency:       "RUB",
	}
	return cfg
}

func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getEnvFloat(key string, def float64) float64 {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func getEnvBool(key string, def bool) bool {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
