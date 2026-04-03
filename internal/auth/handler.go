package auth

import (
	"encoding/json"
	"errors"
	"net/http"
)

type Handler struct {
	sessions *SessionManager
}

func NewHandler(sessions *SessionManager) *Handler {
	return &Handler{sessions: sessions}
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	token, err := h.sessions.RefreshSession(r.Context(), w, r)
	if err != nil {
		if errors.Is(err, ErrRefreshTokenNotFound) || errors.Is(err, ErrRefreshTokenExpired) || errors.Is(err, ErrRefreshTokenRevoked) {
			clearRefreshCookie(w, h.sessions.cfg)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"token":        token,
		"access_token": token,
	})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if err := h.sessions.Logout(r.Context(), w, r); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
