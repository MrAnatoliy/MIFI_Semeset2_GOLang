#!/usr/bin/env bash
# Сквозная проверка API: регистрация -> логин -> счета -> пополнение ->
# карта -> оплата -> перевод -> кредит -> график -> аналитика -> прогноз.
#
# Требуется: curl, jq
# Запуск:    ./scripts/smoke.sh [BASE_URL]

set -euo pipefail

BASE="${1:-http://localhost:8080}"
SUFFIX="$(date +%s)$RANDOM"
USER_A="alice_${SUFFIX}"
USER_B="bob_${SUFFIX}"
EMAIL_A="alice_${SUFFIX}@example.com"
EMAIL_B="bob_${SUFFIX}@example.com"
PASS="Passw0rd123"

command -v jq >/dev/null || { echo "нужен jq: apt install jq / brew install jq"; exit 1; }

ok()   { printf '\033[32m✓\033[0m %s\n' "$1"; }
step() { printf '\n\033[1m▸ %s\033[0m\n' "$1"; }
fail() { printf '\033[31m✗ %s\033[0m\n' "$1"; exit 1; }

api() { # api METHOD PATH [BODY] [TOKEN]
  local method="$1" path="$2" body="${3:-}" token="${4:-}"
  local args=(-sS -X "$method" "$BASE$path" -H 'Content-Type: application/json')
  [ -n "$token" ] && args+=(-H "Authorization: Bearer $token")
  [ -n "$body" ]  && args+=(-d "$body")
  curl "${args[@]}"
}

step "Проверка доступности сервиса"
curl -sS --retry 10 --retry-delay 2 --retry-connrefused "$BASE/health" | jq -e '.status == "ok"' >/dev/null \
  || fail "сервис недоступен по адресу $BASE"
ok "GET /health"

step "Регистрация пользователей"
TOKEN_A=$(api POST /register "{\"username\":\"$USER_A\",\"email\":\"$EMAIL_A\",\"password\":\"$PASS\"}" | jq -er '.token')
ok "зарегистрирован $USER_A"
TOKEN_B=$(api POST /register "{\"username\":\"$USER_B\",\"email\":\"$EMAIL_B\",\"password\":\"$PASS\"}" | jq -er '.token')
ok "зарегистрирован $USER_B"

step "Проверка уникальности email/username"
DUP=$(api POST /register "{\"username\":\"$USER_A\",\"email\":\"$EMAIL_A\",\"password\":\"$PASS\"}" | jq -r '.error // empty')
[ -n "$DUP" ] && ok "повторная регистрация отклонена: $DUP" || fail "дубликат пользователя не отклонён"

step "Аутентификация"
TOKEN_A=$(api POST /login "{\"login\":\"$EMAIL_A\",\"password\":\"$PASS\"}" | jq -er '.token')
ok "JWT получен (срок действия 24 ч)"

BAD=$(api POST /login "{\"login\":\"$EMAIL_A\",\"password\":\"wrong-password\"}" | jq -r '.error // empty')
[ -n "$BAD" ] && ok "неверный пароль отклонён" || fail "вход с неверным паролем прошёл"

step "Защита маршрутов"
NOAUTH=$(api GET /accounts | jq -r '.error // empty')
[ -n "$NOAUTH" ] && ok "запрос без токена отклонён: $NOAUTH" || fail "middleware не блокирует анонимные запросы"

step "Создание счетов"
ACC_A=$(api POST /accounts '{}' "$TOKEN_A" | jq -er '.id')
ok "счёт A создан (id=$ACC_A)"
ACC_B=$(api POST /accounts '{}' "$TOKEN_B" | jq -er '.id')
ok "счёт B создан (id=$ACC_B)"

step "Пополнение баланса"
BAL=$(api POST "/accounts/$ACC_A/deposit" '{"amount":50000,"description":"Первичное пополнение"}' "$TOKEN_A" | jq -er '.balance')
ok "баланс счёта A: $BAL RUB"

step "Запрет операций с чужим счётом"
FOREIGN=$(api POST "/accounts/$ACC_B/deposit" '{"amount":100}' "$TOKEN_A" | jq -r '.error // empty')
[ -n "$FOREIGN" ] && ok "пополнение чужого счёта отклонено: $FOREIGN" || fail "проверка прав доступа не сработала"

step "Выпуск карты"
CARD_JSON=$(api POST /cards "{\"account_id\":$ACC_A}" "$TOKEN_A")
CARD_ID=$(echo "$CARD_JSON" | jq -er '.id')
CARD_CVV=$(echo "$CARD_JSON" | jq -er '.cvv')
CARD_NUM=$(echo "$CARD_JSON" | jq -er '.number')
ok "карта выпущена: $CARD_NUM (id=$CARD_ID)"

# Проверка номера по алгоритму Луна на стороне клиента.
python3 - "$CARD_NUM" <<'PY' && ok "номер карты проходит проверку по алгоритму Луна" || fail "номер карты невалиден"
import sys
n = sys.argv[1]
total, double = 0, False
for c in reversed(n):
    d = int(c)
    if double:
        d *= 2
        if d > 9: d -= 9
    total += d
    double = not double
sys.exit(0 if total % 10 == 0 else 1)
PY

step "Просмотр карт"
api GET /cards '' "$TOKEN_A" | jq -e '.[0].number | test("\\*")' >/dev/null \
  && ok "в списке номер маскирован" || fail "номер карты не замаскирован"
api GET "/cards/$CARD_ID?reveal=true" '' "$TOKEN_A" | jq -e '.integrity_ok == true' >/dev/null \
  && ok "расшифровка владельцу выполнена, HMAC-целостность подтверждена" || fail "проверка целостности не прошла"

step "Оплата картой"
api POST "/cards/$CARD_ID/pay" "{\"cvv\":\"$CARD_CVV\",\"amount\":1500,\"merchant\":\"Coffee Shop\"}" "$TOKEN_A" \
  | jq -er '.account.balance' | xargs -I{} echo "  баланс после оплаты: {} RUB"
ok "платёж проведён"

BADCVV=$(api POST "/cards/$CARD_ID/pay" '{"cvv":"000","amount":100}' "$TOKEN_A" | jq -r '.error // empty')
[ -n "$BADCVV" ] && ok "оплата с неверным CVV отклонена" || fail "проверка CVV не сработала"

step "Перевод между счетами"
api POST /transfer "{\"from_account_id\":$ACC_A,\"to_account_id\":$ACC_B,\"amount\":2500,\"description\":\"Возврат долга\"}" "$TOKEN_A" \
  | jq -er '.from_account.balance' | xargs -I{} echo "  остаток на счёте A: {} RUB"
ok "перевод выполнен"

TOOMUCH=$(api POST /transfer "{\"from_account_id\":$ACC_A,\"to_account_id\":$ACC_B,\"amount\":99999999}" "$TOKEN_A" | jq -r '.error // empty')
[ -n "$TOOMUCH" ] && ok "перевод сверх баланса отклонён: $TOOMUCH" || fail "списание ушло в минус"

step "Ключевая ставка ЦБ РФ"
api GET /rate '' "$TOKEN_A" | jq -c '{key_rate, credit_rate, source}'

step "Оформление кредита"
CREDIT=$(api POST /credits "{\"account_id\":$ACC_A,\"amount\":300000,\"term_months\":12}" "$TOKEN_A")
CREDIT_ID=$(echo "$CREDIT" | jq -er '.credit.id')
echo "$CREDIT" | jq -c '{id: .credit.id, rate: .credit.interest_rate, monthly: .credit.monthly_payment, total: .credit.total_payment}'
ok "кредит оформлен (id=$CREDIT_ID)"

step "График платежей"
api GET "/credits/$CREDIT_ID/schedule" '' "$TOKEN_A" \
  | jq -e '.schedule | length == 12' >/dev/null && ok "график содержит 12 платежей" || fail "некорректный график"
api GET "/credits/$CREDIT_ID/schedule" '' "$TOKEN_A" \
  | jq -r '.schedule[:3][] | "  платёж №\(.payment_number) от \(.due_date[:10]): \(.total_amount) RUB (тело \(.principal_amount), проценты \(.interest_amount))"'

step "Аналитика"
api GET /analytics '' "$TOKEN_A" \
  | jq -c '{total_income, total_expense, net, total_balance, credit_load: {active: .credit_load.active_credits, monthly: .credit_load.monthly_payment_sum, remaining: .credit_load.remaining_debt}}'
ok "аналитика получена"

step "Прогноз баланса"
api GET "/accounts/$ACC_A/predict?days=60" '' "$TOKEN_A" \
  | jq -c '{days, current_balance, total_scheduled_payments, final_balance, will_go_negative}'
ok "прогноз построен"

OVER=$(api GET "/accounts/$ACC_A/predict?days=400" '' "$TOKEN_A" | jq -r '.error // empty')
[ -n "$OVER" ] && ok "лимит горизонта прогноза соблюдается: $OVER" || fail "прогноз свыше 365 дней не отклонён"

step "История операций"
api GET '/transactions?limit=5' '' "$TOKEN_A" | jq -r '.[] | "  \(.created_at[:19])  \(.type)  \(.amount) RUB"'

printf '\n\033[32m▪ Все проверки пройдены.\033[0m Письма доступны в MailHog: http://localhost:8025\n'
