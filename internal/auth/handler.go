package auth

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
)

type Handler struct {
	sessions *SessionManager
	resets   *PasswordResetManager
}

func NewHandler(sessions *SessionManager, resets *PasswordResetManager) *Handler {
	return &Handler{sessions: sessions, resets: resets}
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

func (h *Handler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Email == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email is required"})
		return
	}
	if h.resets == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "password reset is not configured"})
		return
	}
	if err := h.resets.SendReset(r.Context(), req.Email); err != nil {
		log.Printf("forgot password send failed for %q: %v", req.Email, err)
		if errors.Is(err, ErrPasswordResetDisabled) {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "password reset is not configured"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"message": "If that account exists, a password reset email has been sent. If you don't see it within a few minutes, please check your spam or junk folder.",
	})
}

func (h *Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Token == "" || req.NewPassword == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token and new_password are required"})
		return
	}
	if len(req.NewPassword) < 8 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "new_password must be at least 8 characters"})
		return
	}
	if h.resets == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "password reset is not configured"})
		return
	}
	if err := h.resets.ResetPassword(r.Context(), req.Token, req.NewPassword); err != nil {
		if errors.Is(err, ErrPasswordResetNotFound) || errors.Is(err, ErrPasswordResetExpired) || errors.Is(err, ErrPasswordResetUsed) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired reset token"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	h.sessions.ClearRefreshCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"message": "password reset successful"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
