package opportunities

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/goosemooz/something-backend/internal/applications"
	"github.com/goosemooz/something-backend/internal/auth"
)

type Handler struct {
	service    *Service
	appService *applications.Service
}

func NewHandler(service *Service, appService *applications.Service) *Handler {
	return &Handler{service: service, appService: appService}
}

type createRequest struct {
	Title         string     `json:"title"`
	Category      string     `json:"category"`
	Difficulty    Difficulty `json:"difficulty"`
	Description   string     `json:"description"`
	Date          time.Time  `json:"date"`
	Duration      float64    `json:"duration"`
	Location      string     `json:"location"`
	MaxSpots      int        `json:"max_spots"`
	Recurring     string     `json:"recurring"`
	DropIn        bool       `json:"drop_in"`
	EventLink     string     `json:"event_link"`
	ResourcesLink string     `json:"resources_link"`
	Tags          []string   `json:"tags"`
}

type updateRequest struct {
	Title         *string     `json:"title"`
	Category      *string     `json:"category"`
	Difficulty    *Difficulty `json:"difficulty"`
	Description   *string     `json:"description"`
	Date          *time.Time  `json:"date"`
	Duration      *float64    `json:"duration"`
	Location      *string     `json:"location"`
	MaxSpots      *int        `json:"max_spots"`
	Recurring     *string     `json:"recurring"`
	DropIn        *bool       `json:"drop_in"`
	EventLink     *string     `json:"event_link"`
	ResourcesLink *string     `json:"resources_link"`
	Tags          []string    `json:"tags"`
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	opps, err := h.service.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, opps)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	opp, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if opp == nil {
		writeError(w, http.StatusNotFound, "opportunity not found")
		return
	}
	writeJSON(w, http.StatusOK, opp)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r)

	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Title == "" || req.Category == "" || req.Description == "" || req.Duration <= 0 || req.Location == "" || req.MaxSpots <= 0 {
		writeError(w, http.StatusBadRequest, "title, category, description, positive duration, location and positive max_spots are required")
		return
	}
	if req.Date.IsZero() {
		writeError(w, http.StatusBadRequest, "date is required")
		return
	}
	if !req.Date.After(time.Now()) {
		writeError(w, http.StatusBadRequest, "date must be in the future")
		return
	}

	opp, err := h.service.Create(r.Context(), claims.UserID, Opportunity{
		Title:         req.Title,
		Category:      req.Category,
		Difficulty:    req.Difficulty,
		Description:   req.Description,
		Date:          req.Date,
		Duration:      req.Duration,
		Location:      req.Location,
		MaxSpots:      req.MaxSpots,
		Recurring:     req.Recurring,
		DropIn:        req.DropIn,
		EventLink:     req.EventLink,
		ResourcesLink: req.ResourcesLink,
		Tags:          req.Tags,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, opp)
}

func (h *Handler) Apply(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r)
	oppID := "opportunities:" + chi.URLParam(r, "id")

	app, err := h.appService.Create(r.Context(), claims.UserID, oppID)
	if err != nil {
		if errors.Is(err, applications.ErrAlreadyApplied) {
			writeError(w, http.StatusConflict, "already applied to this opportunity")
			return
		}
		if errors.Is(err, applications.ErrNotFound) {
			writeError(w, http.StatusNotFound, "opportunity not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, app)
}

func (h *Handler) ListByOrg(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	opps, err := h.service.ListByOrg(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, opps)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := auth.GetClaims(r)

	opp, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if opp == nil {
		writeError(w, http.StatusNotFound, "opportunity not found")
		return
	}
	if opp.OrgID.String() != claims.UserID {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	h.appService.NotifyOpportunityCanceled(r.Context(), id)

	if err := h.service.Delete(r.Context(), id, claims.UserID); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "opportunity not found")
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

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := auth.GetClaims(r)

	var req updateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	updates := map[string]any{}
	if req.Title != nil {
		if *req.Title == "" {
			writeError(w, http.StatusBadRequest, "title cannot be empty")
			return
		}
		updates["title"] = *req.Title
	}
	if req.Category != nil {
		if *req.Category == "" {
			writeError(w, http.StatusBadRequest, "category cannot be empty")
			return
		}
		updates["category"] = *req.Category
	}
	if req.Difficulty != nil {
		updates["difficulty"] = *req.Difficulty
	}
	if req.Description != nil {
		if *req.Description == "" {
			writeError(w, http.StatusBadRequest, "description cannot be empty")
			return
		}
		updates["description"] = *req.Description
	}
	if req.Date != nil {
		if req.Date.IsZero() {
			writeError(w, http.StatusBadRequest, "date is required")
			return
		}
		updates["date"] = *req.Date
	}
	if req.Duration != nil {
		if *req.Duration <= 0 {
			writeError(w, http.StatusBadRequest, "duration must be positive")
			return
		}
		updates["duration"] = *req.Duration
	}
	if req.Location != nil {
		if *req.Location == "" {
			writeError(w, http.StatusBadRequest, "location cannot be empty")
			return
		}
		updates["location"] = *req.Location
	}
	if req.MaxSpots != nil {
		if *req.MaxSpots <= 0 {
			writeError(w, http.StatusBadRequest, "max_spots must be positive")
			return
		}
		updates["max_spots"] = *req.MaxSpots
	}
	if req.Recurring != nil {
		updates["recurring"] = *req.Recurring
	}
	if req.DropIn != nil {
		updates["drop_in"] = *req.DropIn
	}
	if req.EventLink != nil {
		updates["event_link"] = *req.EventLink
	}
	if req.ResourcesLink != nil {
		updates["resources_link"] = *req.ResourcesLink
	}
	if req.Tags != nil {
		updates["tags"] = req.Tags
	}

	if len(updates) == 0 {
		writeError(w, http.StatusBadRequest, "no fields to update")
		return
	}

	opp, err := h.service.Update(r.Context(), id, claims.UserID, updates)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "opportunity not found")
			return
		}
		if errors.Is(err, ErrForbidden) {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		if err.Error() == "max_spots cannot be less than accepted applications" {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, opp)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
