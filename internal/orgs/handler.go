package orgs

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/goosemooz/something-backend/config"
	"github.com/goosemooz/something-backend/internal/auth"
	"github.com/goosemooz/something-backend/internal/storage"
)

const maxPFPSize = 5 << 20 // 5 MB

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
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Location string `json:"location"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type updateRequest struct {
	Name        *string  `json:"name"`
	Description *string  `json:"description"`
	Website     *string  `json:"website"`
	Phone       *string  `json:"phone"`
	Address     *string  `json:"address"`
	Location    *string  `json:"location"`
	Categories  []string `json:"categories"`
	Instagram   *string  `json:"instagram"`
	LinkedIn    *string  `json:"linkedin"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.Email == "" || req.Password == "" || req.Location == "" {
		writeError(w, http.StatusBadRequest, "name, email, password and location are required")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	org, err := h.service.Create(r.Context(), req.Name, hash, req.Email, req.Location)
	if err != nil {
		existing, lookupErr := h.service.GetByEmail(r.Context(), req.Email)
		if lookupErr == nil && existing != nil {
			writeError(w, http.StatusConflict, "email already in use")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, org)
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

	org, err := h.service.GetByEmail(r.Context(), req.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if org == nil || !auth.CheckPassword(org.PasswordHash, req.Password) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	token, err := h.sessions.IssueSession(r.Context(), w, org.ID.String())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"token": token, "access_token": token})
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	orgs, err := h.service.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, orgs)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	org, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if org == nil {
		writeError(w, http.StatusNotFound, "org not found")
		return
	}
	writeJSON(w, http.StatusOK, org)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Verify the logged-in org owns this record
	claims := auth.GetClaims(r)
	if claims.UserID != "orgs:"+id {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	var req updateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Only include fields that were actually provided
	updates := map[string]any{}
	if req.Name != nil {
		if strings.TrimSpace(*req.Name) == "" {
			writeError(w, http.StatusBadRequest, "name cannot be empty")
			return
		}
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Website != nil {
		updates["website"] = *req.Website
	}
	if req.Phone != nil {
		updates["phone"] = *req.Phone
	}
	if req.Address != nil {
		updates["address"] = *req.Address
	}
	if req.Location != nil {
		if strings.TrimSpace(*req.Location) == "" {
			writeError(w, http.StatusBadRequest, "location cannot be empty")
			return
		}
		updates["location"] = *req.Location
	}
	if req.Categories != nil {
		updates["categories"] = req.Categories
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

	org, err := h.service.Update(r.Context(), id, updates)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if org == nil {
		writeError(w, http.StatusNotFound, "org not found")
		return
	}
	writeJSON(w, http.StatusOK, org)
}

func (h *Handler) UploadPFP(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	claims := auth.GetClaims(r)
	if claims.UserID != "orgs:"+id {
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

	buf := make([]byte, 512)
	n, _ := file.Read(buf)
	ct := http.DetectContentType(buf[:n])
	if !isAllowedPFPContentType(ct) {
		writeError(w, http.StatusBadRequest, "file must be a JPG, PNG, GIF, or WebP image")
		return
	}

	ext := contentTypeToExt(ct)
	key := "pfp/orgs/" + id + ext
	rest, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "upload failed")
		return
	}
	body := append(append([]byte{}, buf[:n]...), rest...)

	existingOrg, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if existingOrg != nil && existingOrg.S3PFP != "" {
		if err := h.store.DeleteOwnedURL(r.Context(), existingOrg.S3PFP); err != nil {
			writeError(w, http.StatusInternalServerError, "upload failed")
			return
		}
	}

	url, err := h.store.UploadPFP(r.Context(), key, body, ct)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "upload failed")
		return
	}

	org, err := h.service.Update(r.Context(), id, map[string]any{"s3_pfp": url})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, org)
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
