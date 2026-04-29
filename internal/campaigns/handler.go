package campaigns

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"net/mail"
	"strings"

	"github.com/goosemooz/something-backend/config"
	internalmail "github.com/goosemooz/something-backend/internal/mail"
)

type Handler struct {
	cfg    *config.Config
	mailer internalmail.Mailer
}

func NewHandler(cfg *config.Config, mailer internalmail.Mailer) *Handler {
	return &Handler{cfg: cfg, mailer: mailer}
}

type sendLaunchRequest struct {
	Emails []string `json:"emails"`
}

type sendLaunchResponse struct {
	Sent          int      `json:"sent"`
	Skipped       int      `json:"skipped"`
	InvalidEmails []string `json:"invalid_emails"`
}

func (h *Handler) SendLaunchNotification(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(h.cfg.CampaignAPIKey) == "" {
		writeError(w, http.StatusInternalServerError, "campaign endpoint is not configured")
		return
	}
	if !matchesAPIKey(r.Header.Get("X-Campaign-API-Key"), h.cfg.CampaignAPIKey) {
		writeError(w, http.StatusUnauthorized, "invalid campaign api key")
		return
	}
	if h.mailer == nil {
		writeError(w, http.StatusInternalServerError, "mailer is not configured")
		return
	}
	if strings.TrimSpace(h.cfg.AppBaseURL) == "" {
		writeError(w, http.StatusInternalServerError, "app base url is not configured")
		return
	}

	var req sendLaunchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Emails) == 0 {
		writeError(w, http.StatusBadRequest, "emails are required")
		return
	}

	seen := make(map[string]struct{}, len(req.Emails))
	resp := sendLaunchResponse{
		InvalidEmails: make([]string, 0),
	}

	for _, rawEmail := range req.Emails {
		email := normalizeEmail(rawEmail)
		if email == "" {
			resp.Skipped++
			continue
		}
		if _, ok := seen[email]; ok {
			resp.Skipped++
			continue
		}
		seen[email] = struct{}{}

		if _, err := mail.ParseAddress(email); err != nil {
			resp.InvalidEmails = append(resp.InvalidEmails, email)
			resp.Skipped++
			continue
		}

		if err := h.mailer.SendLaunchNotification(r.Context(), email, h.cfg.AppBaseURL); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to send launch emails")
			return
		}
		resp.Sent++
	}

	writeJSON(w, http.StatusOK, resp)
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func matchesAPIKey(got, want string) bool {
	got = strings.TrimSpace(got)
	want = strings.TrimSpace(want)
	if len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
