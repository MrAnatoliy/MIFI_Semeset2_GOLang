-- Расширение pgcrypto (требование ТЗ): используется для gen_random_uuid()
-- и доступно как резервный механизм криптографии на стороне БД.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
    id            BIGSERIAL PRIMARY KEY,
    username      VARCHAR(64)  NOT NULL UNIQUE,
    email         VARCHAR(255) NOT NULL UNIQUE,
    password_hash TEXT         NOT NULL,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS accounts (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    number     VARCHAR(20)   NOT NULL UNIQUE,
    balance    NUMERIC(18,2) NOT NULL DEFAULT 0 CHECK (balance >= 0),
    currency   CHAR(3)       NOT NULL DEFAULT 'RUB' CHECK (currency = 'RUB'),
    created_at TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_accounts_user_id ON accounts(user_id);

CREATE TABLE IF NOT EXISTS cards (
    id               BIGSERIAL PRIMARY KEY,
    account_id       BIGINT      NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    user_id          BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    number_encrypted BYTEA       NOT NULL,           -- PGP
    expiry_encrypted BYTEA       NOT NULL,           -- PGP
    number_hmac      TEXT        NOT NULL UNIQUE,    -- HMAC-SHA256, контроль целостности
    cvv_hash         TEXT        NOT NULL,           -- bcrypt
    last4            CHAR(4)     NOT NULL,
    status           VARCHAR(16) NOT NULL DEFAULT 'active',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_cards_user_id ON cards(user_id);
CREATE INDEX IF NOT EXISTS idx_cards_account_id ON cards(account_id);

CREATE TABLE IF NOT EXISTS transactions (
    id                      BIGSERIAL PRIMARY KEY,
    account_id              BIGINT        NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    counterparty_account_id BIGINT        REFERENCES accounts(id) ON DELETE SET NULL,
    type                    VARCHAR(24)   NOT NULL,
    amount                  NUMERIC(18,2) NOT NULL CHECK (amount > 0),
    description             TEXT          NOT NULL DEFAULT '',
    created_at              TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_tx_account_created ON transactions(account_id, created_at DESC);

CREATE TABLE IF NOT EXISTS credits (
    id              BIGSERIAL PRIMARY KEY,
    user_id         BIGINT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id      BIGINT        NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    principal       NUMERIC(18,2) NOT NULL CHECK (principal > 0),
    interest_rate   NUMERIC(6,2)  NOT NULL,
    term_months     INT           NOT NULL CHECK (term_months > 0),
    monthly_payment NUMERIC(18,2) NOT NULL,
    total_payment   NUMERIC(18,2) NOT NULL,
    status          VARCHAR(16)   NOT NULL DEFAULT 'active',
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_credits_user_id ON credits(user_id);

CREATE TABLE IF NOT EXISTS payment_schedules (
    id               BIGSERIAL PRIMARY KEY,
    credit_id        BIGINT        NOT NULL REFERENCES credits(id) ON DELETE CASCADE,
    payment_number   INT           NOT NULL,
    due_date         DATE          NOT NULL,
    total_amount     NUMERIC(18,2) NOT NULL,
    principal_amount NUMERIC(18,2) NOT NULL,
    interest_amount  NUMERIC(18,2) NOT NULL,
    penalty_amount   NUMERIC(18,2) NOT NULL DEFAULT 0,
    status           VARCHAR(16)   NOT NULL DEFAULT 'pending',
    paid_at          TIMESTAMPTZ,
    UNIQUE (credit_id, payment_number)
);
CREATE INDEX IF NOT EXISTS idx_sched_due ON payment_schedules(status, due_date);
