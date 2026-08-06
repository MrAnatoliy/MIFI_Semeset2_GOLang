package handler

import (
	"net/http"

	"bankapi/internal/models"
)

// CreateCard - POST /cards.
func (h *Handler) CreateCard(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	var req models.CreateCardRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	card, err := h.svc.Card.Issue(r.Context(), userID, req.AccountID)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, card)
}

// ListCards - GET /cards (номера маскированы).
func (h *Handler) ListCards(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	cards, err := h.svc.Card.List(r.Context(), userID)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cards)
}

// GetCard - GET /cards/{cardId}?reveal=true - расшифровка для владельца.
func (h *Handler) GetCard(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	cardID, err := pathInt64(r, "cardId")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	reveal := r.URL.Query().Get("reveal") == "true"
	card, err := h.svc.Card.Get(r.Context(), userID, cardID, reveal)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, card)
}

// PayWithCard - POST /cards/{cardId}/pay.
func (h *Handler) PayWithCard(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	cardID, err := pathInt64(r, "cardId")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req models.CardPaymentRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	acc, err := h.svc.Card.Pay(r.Context(), userID, cardID, &req)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "платёж проведён",
		"account": acc,
	})
}

// BlockCard - POST /cards/{cardId}/block.
func (h *Handler) BlockCard(w http.ResponseWriter, r *http.Request) {
	h.setCardStatus(w, r, models.StatusBlocked)
}

// UnblockCard - POST /cards/{cardId}/unblock.
func (h *Handler) UnblockCard(w http.ResponseWriter, r *http.Request) {
	h.setCardStatus(w, r, models.StatusActive)
}

func (h *Handler) setCardStatus(w http.ResponseWriter, r *http.Request, status string) {
	userID, _ := h.userID(r)
	cardID, err := pathInt64(r, "cardId")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.svc.Card.SetStatus(r.Context(), userID, cardID, status); err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": status})
}
