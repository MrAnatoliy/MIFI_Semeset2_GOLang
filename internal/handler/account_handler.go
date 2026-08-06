package handler

import (
	"net/http"
	"strconv"

	"bankapi/internal/models"
)

// CreateAccount - POST /accounts.
func (h *Handler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	acc, err := h.svc.Account.Create(r.Context(), userID)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, acc)
}

// ListAccounts - GET /accounts.
func (h *Handler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	accounts, err := h.svc.Account.List(r.Context(), userID)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, accounts)
}

// GetAccount - GET /accounts/{accountId}.
func (h *Handler) GetAccount(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	accountID, err := pathInt64(r, "accountId")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	acc, err := h.svc.Account.GetOwned(r.Context(), nil, accountID, userID)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, acc)
}

// Deposit - POST /accounts/{accountId}/deposit.
func (h *Handler) Deposit(w http.ResponseWriter, r *http.Request) {
	h.amountOperation(w, r, true)
}

// Withdraw - POST /accounts/{accountId}/withdraw.
func (h *Handler) Withdraw(w http.ResponseWriter, r *http.Request) {
	h.amountOperation(w, r, false)
}

func (h *Handler) amountOperation(w http.ResponseWriter, r *http.Request, deposit bool) {
	userID, _ := h.userID(r)
	accountID, err := pathInt64(r, "accountId")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req models.AmountRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var acc *models.Account
	if deposit {
		acc, err = h.svc.Account.Deposit(r.Context(), userID, accountID, req.Amount, req.Description)
	} else {
		acc, err = h.svc.Account.Withdraw(r.Context(), userID, accountID, req.Amount, req.Description)
	}
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, acc)
}

// Transfer - POST /transfer.
func (h *Handler) Transfer(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	var req models.TransferRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	acc, err := h.svc.Account.Transfer(r.Context(), userID, &req)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":      "перевод выполнен",
		"from_account": acc,
	})
}

// History - GET /transactions?limit=N.
func (h *Handler) History(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	list, err := h.svc.Account.History(r.Context(), userID, limit)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}
