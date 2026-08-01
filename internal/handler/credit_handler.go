package handler

import (
	"net/http"
	"strconv"
	"time"

	"bankapi/internal/models"
)

// CreateCredit — POST /credits.
func (h *Handler) CreateCredit(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	var req models.CreateCreditRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := h.svc.Credit.Create(r.Context(), userID, &req)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

// ListCredits — GET /credits.
func (h *Handler) ListCredits(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	credits, err := h.svc.Credit.List(r.Context(), userID)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, credits)
}

// CreditSchedule — GET /credits/{creditId}/schedule.
func (h *Handler) CreditSchedule(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	creditID, err := pathInt64(r, "creditId")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := h.svc.Credit.Schedule(r.Context(), userID, creditID)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// Analytics — GET /analytics?from=YYYY-MM-DD&to=YYYY-MM-DD.
func (h *Handler) Analytics(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)

	to := time.Now().Add(24 * time.Hour)
	from := time.Now().AddDate(-1, 0, 0)

	if raw := r.URL.Query().Get("from"); raw != "" {
		t, err := time.Parse("2006-01-02", raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "параметр from: ожидается формат YYYY-MM-DD")
			return
		}
		from = t
	}
	if raw := r.URL.Query().Get("to"); raw != "" {
		t, err := time.Parse("2006-01-02", raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "параметр to: ожидается формат YYYY-MM-DD")
			return
		}
		to = t.AddDate(0, 0, 1) // включаем указанный день целиком
	}
	if !from.Before(to) {
		writeError(w, http.StatusBadRequest, "период задан некорректно: from должен быть раньше to")
		return
	}

	res, err := h.svc.Analytics.Overview(r.Context(), userID, from, to)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// PredictBalance — GET /accounts/{accountId}/predict?days=N.
func (h *Handler) PredictBalance(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	accountID, err := pathInt64(r, "accountId")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	days := 30
	if raw := r.URL.Query().Get("days"); raw != "" {
		n, convErr := strconv.Atoi(raw)
		if convErr != nil {
			writeError(w, http.StatusBadRequest, "параметр days: ожидается целое число")
			return
		}
		days = n
	}
	res, err := h.svc.Analytics.Predict(r.Context(), userID, accountID, days)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
