package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"bankapi/internal/config"
	"bankapi/internal/middleware"
	"bankapi/internal/service"
)

// Handler держит зависимости HTTP-слоя.
type Handler struct {
	svc *service.Services
	cfg *config.Config
	log *logrus.Logger
}

// New создаёт обработчики запросов.
func New(svc *service.Services, cfg *config.Config, log *logrus.Logger) *Handler {
	return &Handler{svc: svc, cfg: cfg, log: log}
}

// --- Вспомогательные функции ответа ---

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if payload != nil {
		_ = json.NewEncoder(w).Encode(payload)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// decodeJSON разбирает тело запроса с ограничением размера и запретом лишних полей.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst interface{}) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("тело запроса пустое")
		}
		return errors.New("некорректный JSON: " + err.Error())
	}
	return nil
}

// pathInt64 извлекает числовой параметр маршрута.
func pathInt64(r *http.Request, name string) (int64, error) {
	raw := mux.Vars(r)[name]
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("некорректный параметр " + name)
	}
	return id, nil
}

// userID возвращает ID пользователя, установленный middleware аутентификации.
func (h *Handler) userID(r *http.Request) (int64, bool) {
	return middleware.UserID(r.Context())
}

// handleServiceError переводит доменные ошибки в HTTP-статусы.
func (h *Handler) handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrUserExists):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrInvalidCredential):
		writeError(w, http.StatusUnauthorized, err.Error())
	case errors.Is(err, service.ErrForbidden):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, service.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrInsufficientFunds),
		errors.Is(err, service.ErrCardExpired),
		errors.Is(err, service.ErrCardBlocked),
		errors.Is(err, service.ErrCurrency),
		errors.Is(err, service.ErrPredictRange):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, service.ErrInvalidCVV):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, service.ErrIntegrity):
		writeError(w, http.StatusInternalServerError, err.Error())
	default:
		h.log.Errorf("необработанная ошибка: %v", err)
		writeError(w, http.StatusInternalServerError, "внутренняя ошибка сервера")
	}
}

// Health — проверка живости сервиса.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// KeyRate возвращает текущую ключевую ставку ЦБ РФ и ставку кредитования банка.
func (h *Handler) KeyRate(w http.ResponseWriter, r *http.Request) {
	rate, err := h.svc.CBR.KeyRate(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"key_rate":    nil,
			"credit_rate": h.svc.CBR.CreditRate(r.Context()),
			"source":      "fallback",
			"warning":     "сервис ЦБ РФ временно недоступен",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"key_rate":    rate,
		"credit_rate": h.svc.CBR.CreditRate(r.Context()),
		"source":      "cbr.ru",
	})
}
