package handler

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"bankapi/internal/config"
	"bankapi/internal/middleware"
	"bankapi/internal/service"
)

// NewRouter собирает маршрутизацию: публичные и защищённые JWT эндпоинты.
func NewRouter(svc *service.Services, cfg *config.Config, log *logrus.Logger) http.Handler {
	h := New(svc, cfg, log)
	r := mux.NewRouter()

	r.Use(middleware.Recover(log))
	r.Use(middleware.Logging(log))
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.ContentJSON)

	// --- Публичные маршруты ---
	r.HandleFunc("/health", h.Health).Methods(http.MethodGet)
	r.HandleFunc("/register", h.Register).Methods(http.MethodPost)
	r.HandleFunc("/login", h.Login).Methods(http.MethodPost)

	// --- Защищённые маршруты (требуют JWT) ---
	api := r.PathPrefix("/").Subrouter()
	api.Use(middleware.Auth(svc.Tokens))

	api.HandleFunc("/me", h.Me).Methods(http.MethodGet)
	api.HandleFunc("/rate", h.KeyRate).Methods(http.MethodGet)

	// Счета
	api.HandleFunc("/accounts", h.CreateAccount).Methods(http.MethodPost)
	api.HandleFunc("/accounts", h.ListAccounts).Methods(http.MethodGet)
	api.HandleFunc("/accounts/{accountId:[0-9]+}", h.GetAccount).Methods(http.MethodGet)
	api.HandleFunc("/accounts/{accountId:[0-9]+}/deposit", h.Deposit).Methods(http.MethodPost)
	api.HandleFunc("/accounts/{accountId:[0-9]+}/withdraw", h.Withdraw).Methods(http.MethodPost)
	api.HandleFunc("/accounts/{accountId:[0-9]+}/predict", h.PredictBalance).Methods(http.MethodGet)

	// Переводы и история
	api.HandleFunc("/transfer", h.Transfer).Methods(http.MethodPost)
	api.HandleFunc("/transactions", h.History).Methods(http.MethodGet)

	// Карты
	api.HandleFunc("/cards", h.CreateCard).Methods(http.MethodPost)
	api.HandleFunc("/cards", h.ListCards).Methods(http.MethodGet)
	api.HandleFunc("/cards/{cardId:[0-9]+}", h.GetCard).Methods(http.MethodGet)
	api.HandleFunc("/cards/{cardId:[0-9]+}/pay", h.PayWithCard).Methods(http.MethodPost)
	api.HandleFunc("/cards/{cardId:[0-9]+}/block", h.BlockCard).Methods(http.MethodPost)
	api.HandleFunc("/cards/{cardId:[0-9]+}/unblock", h.UnblockCard).Methods(http.MethodPost)

	// Кредиты
	api.HandleFunc("/credits", h.CreateCredit).Methods(http.MethodPost)
	api.HandleFunc("/credits", h.ListCredits).Methods(http.MethodGet)
	api.HandleFunc("/credits/{creditId:[0-9]+}/schedule", h.CreditSchedule).Methods(http.MethodGet)

	// Аналитика
	api.HandleFunc("/analytics", h.Analytics).Methods(http.MethodGet)

	notFound := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		writeError(w, http.StatusNotFound, "маршрут не найден")
	})
	r.NotFoundHandler = notFound
	api.NotFoundHandler = notFound
	r.MethodNotAllowedHandler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		writeError(w, http.StatusMethodNotAllowed, "метод не поддерживается")
	})

	return r
}
