# Bank API - REST API банковского сервиса на Go

Контейнеризированное многослойное приложение: регистрация и аутентификация (JWT),
счета и переводы, виртуальные карты с PGP-шифрованием, кредиты с аннуитетными
платежами и автосписанием, финансовая аналитика, интеграции с ЦБ РФ (SOAP) и SMTP.

## Стек

| Компонент| Технология|
|-|-|
| Язык| Go 1.23|
| Маршрутизация| gorilla/mux|
| БД| PostgreSQL 17 + pgcrypto, драйвер lib/pq|
| Аутентификация | JWT (golang-jwt/jwt/v5), срок действия 24 ч |
| Логирование| logrus (JSON-формат)|
| Криптография| bcrypt, HMAC-SHA256, PGP (симметрично, AES-256) |
| Почта| gopkg.in/mail.v2 + MailHog для разработки|
| XML| beevik/etree|

## Быстрый старт

```bash
cp .env.example .env        # при желании поправьте секреты
docker compose up --build -d
docker compose logs -f api
```

Поднимутся три сервиса:

| Сервис| Адрес| Назначение|
|-|-|-|
| api| http://localhost:8080| REST API|
| db| localhost:5432| PostgreSQL 17|
| mailhog | http://localhost:8025| Веб-интерфейс перехваченных писем|

Миграции применяются автоматически при старте (таблица `schema_migrations`
хранит уже применённые файлы, повторный запуск идемпотентен).

Проверка:

```bash
curl -s localhost:8080/health        # {"status":"ok"}
./scripts/smoke.sh                   # сквозной сценарий (нужны curl, jq, python3)
make test                            # юнит-тесты (нужен локальный Go)
```

## Архитектура

```
cmd/server/main.goточка входа, DI, graceful shutdown
internal/
config/конфигурация из переменных окружения
models/структуры данных + DTO с валидацией
repository/ SQL-запросы, транзакции, миграции
service/ бизнес-логика, интеграции, шедулер
handler/ HTTP-обработчики и маршрутизация
middleware/ JWT, логирование, recover, security-заголовки
security/bcrypt, PGP, HMAC, алгоритм Луна, JWT
migrations/SQL-миграции
```

Поток запроса: `handler` (валидация, HTTP-коды) => `service` (бизнес-правила,
транзакции, интеграции) => `repository` (параметризованный SQL) => PostgreSQL.
Обратные зависимости отсутствуют

## Эндпоинты

### Публичные

| Метод | Путь| Описание|
|-|-|-|
| GET| `/health`| Проверка живости|
| POST| `/register` | Регистрация (уникальность email и username) |
| POST| `/login` | Аутентификация, выдача JWT |

### Защищённые (заголовок `Authorization: Bearer <token>`)

| Метод | Путь| Описание|
|-|-|-|
| GET| `/me`| Профиль текущего пользователя |
| GET| `/rate`| Ключевая ставка ЦБ РФ и ставка банка|
| POST| `/accounts`| Создать счёт|
| GET| `/accounts`| Список счетов |
| GET| `/accounts/{accountId}`| Счёт по идентификатору|
| POST| `/accounts/{accountId}/deposit` | Пополнение |
| POST| `/accounts/{accountId}/withdraw`| Списание|
| GET| `/accounts/{accountId}/predict?days=N`| Прогноз баланса (1 -365 дней)|
| POST| `/transfer`| Перевод между счетами|
| GET| `/transactions?limit=N`| История операций|
| POST| `/cards`| Выпуск виртуальной карты|
| GET| `/cards`| Список карт (номера маскированы) |
| GET| `/cards/{cardId}?reveal=true`| Карта с расшифровкой для владельца|
| POST| `/cards/{cardId}/pay`| Оплата картой (подтверждение по CVV)|
| POST| `/cards/{cardId}/block`| Блокировка карты|
| POST| `/cards/{cardId}/unblock` | Разблокировка карты|
| POST| `/credits`| Оформление кредита|
| GET| `/credits`| Список кредитов|
| GET| `/credits/{creditId}/schedule`| График платежей|
| GET| `/analytics?from=&to=` | Доходы/расходы, кредитная нагрузка|

### Примеры

```bash
# Регистрация
curl -s -X POST localhost:8080/register -H 'Content-Type: application/json' \
  -d '{"username":"alice","email":"alice@example.com","password":"Passw0rd123"}'

TOKEN=$(curl -s -X POST localhost:8080/login -H 'Content-Type: application/json' \
  -d '{"login":"alice@example.com","password":"Passw0rd123"}' | jq -r .token)

# Счёт и пополнение
ACC=$(curl -s -X POST localhost:8080/accounts -H "Authorization: Bearer $TOKEN" | jq -r .id)
curl -s -X POST localhost:8080/accounts/$ACC/deposit \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"amount":50000,"description":"Зарплата"}'

# Выпуск карты - открытые данные возвращаются ОДИН раз
curl -s -X POST localhost:8080/cards \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d "{\"account_id\":$ACC}"

# Кредит на 300 000 RUB на 12 месяцев
curl -s -X POST localhost:8080/credits \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d "{\"account_id\":$ACC,\"amount\":300000,\"term_months\":12}"
```

Коды ответов: `400` - ошибка валидации, `401` - нет/просрочен токен,
`403` - чужой ресурс или неверный CVV, `404` - не найдено, `409` - дубликат
пользователя, `422` - бизнес-ограничение (нехватка средств, истёкшая карта,
горизонт прогноза), `500` - внутренняя ошибка.

## Безопасность

- **Пароли** - bcrypt (`DefaultCost`), в ответах API никогда не фигурируют.
- **Номер и срок карты** - симметричное PGP-шифрование AES-256 на парольной
фразе `PGP_PASSPHRASE`, хранятся как `bytea`.
- **CVV** - bcrypt-хеш; открытое значение показывается один раз при выпуске.
- **Целостность карты** - HMAC-SHA256 от пары  "номер|срок ", сверка выполняется
в постоянном времени (`hmac.Equal`) при каждом чтении и оплате.
- **JWT** - HS256, секрет `JWT_SECRET`, срок действия 24 часа, проверка issuer
и допустимого алгоритма подписи (защита от подмены на `none`).
- **Права доступа** - каждая операция сверяет владельца счёта или карты;
пополнение чужого счёта и просмотр чужой карты запрещены.
- **SQL-инъекции** - только параметризованные запросы, конкатенации нет.
- **Целостность денег** - переводы в транзакции с `SELECT ... FOR UPDATE`,
счета блокируются по возрастанию ID (защита от взаимоблокировок),
ограничение `CHECK (balance >= 0)` на уровне БД.
- **HTTP** - ограничение тела запроса 1 МБ, `DisallowUnknownFields`,
заголовки `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`,
контейнер работает от непривилегированного пользователя.

## Бизнес-логика

**Кредиты.** Ставка = ключевая ставка ЦБ РФ + маржа банка (`BANK_MARGIN`,
по умолчанию 5 п.п.), с ограничением коридором `MIN_CREDIT_RATE`…`MAX_CREDIT_RATE`.
Аннуитетный платёж:

```
P = S * i * (1+i)^n / ((1+i)^n  - 1),i = годовая ставка / 12 / 100
```

График строится с разбивкой на тело и проценты; последний платёж закрывает
остаток, поэтому накопленная погрешность округления не переносится на клиента.

**Шедулер.** Раз в `SCHEDULER_INTERVAL_HOURS` (по умолчанию 12) выбирает
неоплаченные платежи со сроком `due_date <= now()` и списывает их со счёта.
При нехватке средств начисляется штраф `+10%` от суммы платежа - однократно
(условие `WHERE penalty_amount = 0`), статус переводится в `overdue`,
пользователю уходит письмо. На следующих прогонах списание повторяется без
повторного штрафа. Когда непогашенных платежей не остаётся, кредит
автоматически переходит в статус `closed`.

**Аналитика.** Доходы (`deposit`, `transfer_in`, `credit_issue`) и расходы
(`withdrawal`, `transfer_out`, `card_payment`, `credit_payment`) в разрезе
месяцев и типов операций, кредитная нагрузка с показателем долговой нагрузки
(платежи / средний доход), прогноз баланса по дням с учётом графика платежей.

**Интеграции.** Ключевая ставка запрашивается SOAP-методом `KeyRate` у
`cbr.ru/DailyInfoWebServ/DailyInfo.asmx`, ответ разбирается через `etree`,
результат кешируется на час. Если сервис ЦБ недоступен, используется резервное
значение `FALLBACK_KEY_RATE` - оформление кредита не блокируется. Письма
отправляются асинхронно: отказ SMTP не прерывает банковскую операцию.

## Переменные окружения

| Переменная| По умолчанию| Назначение|
|-|-|-|
| `APP_PORT`| `8080`| Порт HTTP-сервера |
| `LOG_LEVEL` | `info`| `debug`/`info`/`warn`/`error` |
| `DB_HOST`…`DB_SSLMODE`| см. `.env.example`| Подключение к PostgreSQL|
| `JWT_SECRET`| -| Секрет подписи токенов|
| `JWT_TTL_HOURS`| `24`| Срок действия токена |
| `HMAC_SECRET`| -| Секрет HMAC для карт |
| `PGP_PASSPHRASE`| -| Парольная фраза PGP|
| `SMTP_*` | MailHog| Параметры почтового сервера|
| `SCHEDULER_INTERVAL_HOURS` | `12`| Периодичность обработки платежей |
| `BANK_MARGIN`| `5`| Маржа банка, п.п. |
| `FALLBACK_KEY_RATE`| `16`| Ставка при недоступности ЦБ РФ|
| `PENALTY_RATE` | `0.10`| Штраф за просрочку|
| `MAX_PREDICT_DAYS`| `365` | Горизонт прогноза баланса|

Смена `PGP_PASSPHRASE` сделает ранее выпущенные карты нечитаемыми, а смена `HMAC_SECRET` - уронит проверку их целостности.

## Ограничения

- Единственная поддерживаемая валюта - RUB (проверяется и в БД, и в сервисах).
- Горизонт прогноза баланса - не более 365 дней.
- 2FA и административная панель не реализованы (опциональная часть ТЗ).

## Команды

```bash
make up      # поднять стек
make logs    # логи API
make test    # юнит-тесты
make psql    # консоль PostgreSQL
make down    # остановить
make clean   # остановить и удалить том с данными
```
