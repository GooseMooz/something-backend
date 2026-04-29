package users

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/goosemooz/something-backend/config"
	"github.com/goosemooz/something-backend/internal/auth"
	"github.com/goosemooz/something-backend/internal/storage"
)

const (
	maxPFPSize    = 5 << 20  // 5 MB
	maxResumeSize = 10 << 20 // 10 MB
)

type Handler struct {
	service  *Service
	cfg      *config.Config
	store    *storage.Storage
	sessions *auth.SessionManager
}

func NewHandler(service *Service, cfg *config.Config, store *storage.Storage, sessions *auth.SessionManager) *Handler {
	return &Handler{service: service, cfg: cfg, store: store, sessions: sessions}
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type updateRequest struct {
	Name       *string  `json:"name"`
	Bio        *string  `json:"bio"`
	Skills     []string `json:"skills"`
	Categories []string `json:"categories"`
	Intensity  *string  `json:"intensity"`
	Phone      *string  `json:"phone"`
	Instagram  *string  `json:"instagram"`
	LinkedIn   *string  `json:"linkedin"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type updateNotificationSettingsRequest struct {
	ApplicationAccepted *bool `json:"application_accepted"`
	OpportunityReminder *bool `json:"opportunity_reminder"`
	ApplicationDeclined *bool `json:"application_declined"`
	OpportunityCanceled *bool `json:"opportunity_canceled"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "email, password and name are required")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	user, err := h.service.Create(r.Context(), req.Email, hash, req.Name)
	if err != nil {
		existing, lookupErr := h.service.GetByEmail(r.Context(), req.Email)
		if lookupErr == nil && existing != nil {
			writeError(w, http.StatusConflict, "email already in use")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, user)
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	user, err := h.service.GetByEmail(r.Context(), req.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if user == nil || !auth.CheckPassword(user.PasswordHash, req.Password) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	token, err := h.sessions.IssueSession(r.Context(), w, user.ID.String())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"token": token, "access_token": token})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	user, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if user == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	// Return the full record (including email) only to the owner.
	claims := auth.TryGetClaims(r, h.cfg)
	if claims != nil && claims.UserID == "users:"+id {
		writeJSON(w, http.StatusOK, user)
		return
	}
	writeJSON(w, http.StatusOK, user.Public())
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	claims := auth.GetClaims(r)
	if claims.UserID != "users:"+id {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	var req updateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	updates := map[string]any{}
	if req.Name != nil {
		if strings.TrimSpace(*req.Name) == "" {
			writeError(w, http.StatusBadRequest, "name cannot be empty")
			return
		}
		updates["name"] = *req.Name
	}
	if req.Bio != nil {
		updates["bio"] = *req.Bio
	}
	if req.Skills != nil {
		updates["skills"] = req.Skills
	}
	if req.Categories != nil {
		updates["categories"] = req.Categories
	}
	if req.Intensity != nil {
		if *req.Intensity != "low" && *req.Intensity != "medium" && *req.Intensity != "high" {
			writeError(w, http.StatusBadRequest, "intensity must be low, medium or high")
			return
		}
		updates["intensity"] = *req.Intensity
	}
	if req.Phone != nil {
		updates["phone"] = *req.Phone
	}
	if req.Instagram != nil {
		updates["instagram"] = *req.Instagram
	}
	if req.LinkedIn != nil {
		updates["linkedin"] = *req.LinkedIn
	}

	if len(updates) == 0 {
		writeError(w, http.StatusBadRequest, "no fields to update")
		return
	}

	user, err := h.service.Update(r.Context(), id, updates)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if user == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	claims := auth.GetClaims(r)
	if claims.UserID != "users:"+id {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.CurrentPassword == "" || req.NewPassword == "" {
		writeError(w, http.StatusBadRequest, "current_password and new_password are required")
		return
	}
	if len(req.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "new_password must be at least 8 characters")
		return
	}
	if req.CurrentPassword == req.NewPassword {
		writeError(w, http.StatusBadRequest, "new_password must be different from current_password")
		return
	}

	user, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if user == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if !auth.CheckPassword(user.PasswordHash, req.CurrentPassword) {
		writeError(w, http.StatusUnauthorized, "invalid current password")
		return
	}

	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	updated, err := h.service.Update(r.Context(), id, map[string]any{"password_hash": hash})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if updated == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	if err := h.sessions.RevokeAllForUser(r.Context(), updated.ID.String()); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	h.sessions.ClearRefreshCookie(w)

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "password updated, please sign in again",
	})
}

func (h *Handler) GetNotificationSettings(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	claims := auth.GetClaims(r)
	if claims.UserID != "users:"+id {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	user, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if user == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, user.NotificationSettings)
}

func (h *Handler) UpdateNotificationSettings(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	claims := auth.GetClaims(r)
	if claims.UserID != "users:"+id {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	var req updateNotificationSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if user == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	settings := user.NotificationSettings
	updated := false
	if req.ApplicationAccepted != nil {
		settings.ApplicationAccepted = *req.ApplicationAccepted
		updated = true
	}
	if req.OpportunityReminder != nil {
		settings.OpportunityReminder = *req.OpportunityReminder
		updated = true
	}
	if req.ApplicationDeclined != nil {
		settings.ApplicationDeclined = *req.ApplicationDeclined
		updated = true
	}
	if req.OpportunityCanceled != nil {
		settings.OpportunityCanceled = *req.OpportunityCanceled
		updated = true
	}
	if !updated {
		writeError(w, http.StatusBadRequest, "no fields to update")
		return
	}

	user, err = h.service.Update(r.Context(), id, map[string]any{"notification_settings": settings})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, user.NotificationSettings)
}

func (h *Handler) UploadPFP(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	claims := auth.GetClaims(r)
	if claims.UserID != "users:"+id {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxPFPSize)
	if err := r.ParseMultipartForm(maxPFPSize); err != nil {
		writeError(w, http.StatusBadRequest, "file too large or invalid form")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file field required")
		return
	}
	defer func() { _ = file.Close() }()

	// Detect actual content type from file bytes
	buf := make([]byte, 512)
	n, _ := file.Read(buf)
	ct := http.DetectContentType(buf[:n])
	if !isAllowedPFPContentType(ct) {
		writeError(w, http.StatusBadRequest, "file must be a JPG, PNG, GIF, or WebP image")
		return
	}

	ext := contentTypeToExt(ct)
	key := "pfp/users/" + id + ext
	rest, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "upload failed")
		return
	}
	body := append(append([]byte{}, buf[:n]...), rest...)

	existingUser, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if existingUser != nil && existingUser.S3PFP != "" {
		if err := h.store.DeleteOwnedURL(r.Context(), existingUser.S3PFP); err != nil {
			writeError(w, http.StatusInternalServerError, "upload failed")
			return
		}
	}

	url, err := h.store.UploadPFP(r.Context(), key, body, ct)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "upload failed")
		return
	}

	user, err := h.service.Update(r.Context(), id, map[string]any{"s3_pfp": url})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (h *Handler) UploadResume(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	claims := auth.GetClaims(r)
	if claims.UserID != "users:"+id {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxResumeSize)
	if err := r.ParseMultipartForm(maxResumeSize); err != nil {
		writeError(w, http.StatusBadRequest, "file too large or invalid form")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file field required")
		return
	}
	defer func() { _ = file.Close() }()

	buf := make([]byte, 512)
	n, _ := file.Read(buf)
	ct := http.DetectContentType(buf[:n])
	if ct != "application/pdf" || !bytes.HasPrefix(buf[:n], []byte("%PDF-")) {
		writeError(w, http.StatusBadRequest, "file must be a PDF")
		return
	}

	key := "resume/users/" + id + ".pdf"
	rest, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "upload failed")
		return
	}
	body := append(append([]byte{}, buf[:n]...), rest...)

	existingUser, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if existingUser != nil && existingUser.S3PDF != "" {
		if err := h.store.DeleteOwnedURL(r.Context(), existingUser.S3PDF); err != nil {
			writeError(w, http.StatusInternalServerError, "upload failed")
			return
		}
	}

	url, err := h.store.UploadPDF(r.Context(), key, body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "upload failed")
		return
	}

	user, err := h.service.Update(r.Context(), id, map[string]any{"s3_pdf": url})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func contentTypeToExt(ct string) string {
	switch ct {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".jpg"
	}
}

func isAllowedPFPContentType(ct string) bool {
	switch ct {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
