package applications

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/goosemooz/something-backend/internal/auth"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

type updateStatusRequest struct {
	Status Status `json:"status"`
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r)
	apps, err := h.service.ListByUser(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, apps)
}

func (h *Handler) ListByOrg(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r)
	id := chi.URLParam(r, "id")
	if claims == nil || claims.UserID != "orgs:"+id {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	apps, err := h.service.ListDetailedByOrg(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, apps)
}

func (h *Handler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r)
	if !strings.HasPrefix(claims.UserID, "orgs:") {
		writeError(w, http.StatusForbidden, "orgs only")
		return
	}

	id := chi.URLParam(r, "id")

	var req updateStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Status != StatusAccepted && req.Status != StatusRejected {
		writeError(w, http.StatusBadRequest, "status must be 'accepted' or 'rejected'")
		return
	}

	app, err := h.service.UpdateStatusDetailed(r.Context(), id, claims.UserID, req.Status)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "application not found")
			return
		}
		if errors.Is(err, ErrForbidden) {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		if errors.Is(err, ErrNoSpotsLeft) {
			writeError(w, http.StatusConflict, "no spots left")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, app)
}

// Delete withdraws the caller's application.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := auth.GetClaims(r)

	if err := h.service.Delete(r.Context(), id, claims.UserID); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "application not found")
			return
		}
		if errors.Is(err, ErrForbidden) {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
