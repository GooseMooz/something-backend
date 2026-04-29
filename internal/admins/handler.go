package admins

import (
	"encoding/json"
	"errors"
	"net/http"
	netmail "net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/goosemooz/something-backend/internal/applications"
	"github.com/goosemooz/something-backend/internal/auth"
	internalmail "github.com/goosemooz/something-backend/internal/mail"
	"github.com/goosemooz/something-backend/internal/opportunities"
	"github.com/goosemooz/something-backend/internal/orgs"
	"github.com/goosemooz/something-backend/internal/users"
)

type Handler struct {
	service    *Service
	userSvc    *users.Service
	orgSvc     *orgs.Service
	oppSvc     *opportunities.Service
	appSvc     *applications.Service
	mailer     internalmail.Mailer
	sessionMgr *auth.SessionManager
}

func NewHandler(service *Service, userSvc *users.Service, orgSvc *orgs.Service, oppSvc *opportunities.Service, appSvc *applications.Service, mailer internalmail.Mailer, sessionMgr *auth.SessionManager) *Handler {
	return &Handler{
		service:    service,
		userSvc:    userSvc,
		orgSvc:     orgSvc,
		oppSvc:     oppSvc,
		appSvc:     appSvc,
		mailer:     mailer,
		sessionMgr: sessionMgr,
	}
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type setOrgVerificationRequest struct {
	Verified *bool `json:"verified"`
}

type updateStatusRequest struct {
	Status applications.Status `json:"status"`
}

type opportunityCreateRequest struct {
	OrgID         string                   `json:"org_id"`
	Title         string                   `json:"title"`
	Category      string                   `json:"category"`
	Difficulty    opportunities.Difficulty `json:"difficulty"`
	Description   string                   `json:"description"`
	Date          time.Time                `json:"date"`
	Duration      float64                  `json:"duration"`
	Location      string                   `json:"location"`
	MaxSpots      int                      `json:"max_spots"`
	Recurring     string                   `json:"recurring"`
	DropIn        bool                     `json:"drop_in"`
	EventLink     string                   `json:"event_link"`
	ResourcesLink string                   `json:"resources_link"`
	Tags          []string                 `json:"tags"`
}

type opportunityUpdateRequest struct {
	Title         *string                   `json:"title"`
	Category      *string                   `json:"category"`
	Difficulty    *opportunities.Difficulty `json:"difficulty"`
	Description   *string                   `json:"description"`
	Date          *time.Time                `json:"date"`
	Duration      *float64                  `json:"duration"`
	Location      *string                   `json:"location"`
	MaxSpots      *int                      `json:"max_spots"`
	Recurring     *string                   `json:"recurring"`
	DropIn        *bool                     `json:"drop_in"`
	EventLink     *string                   `json:"event_link"`
	ResourcesLink *string                   `json:"resources_link"`
	Tags          []string                  `json:"tags"`
}

type opportunityApplicationsResponse struct {
	Opportunity  *opportunities.Opportunity    `json:"opportunity"`
	Applications []applications.OrgApplication `json:"applications"`
}

type sendCampaignRequest struct {
	Audience string `json:"audience"`
	Subject  string `json:"subject"`
	Body     string `json:"body"`
}

type sendCampaignResponse struct {
	Sent          int      `json:"sent"`
	Skipped       int      `json:"skipped"`
	InvalidEmails []string `json:"invalid_emails"`
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Email) == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	admin, err := h.service.GetByEmail(r.Context(), strings.TrimSpace(req.Email))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if admin == nil || !auth.CheckPassword(admin.PasswordHash, req.Password) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	token, err := h.sessionMgr.IssueSession(r.Context(), w, admin.ID.String())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"token":        token,
		"access_token": token,
		"admin":        admin,
	})
}

func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	allUsers, err := h.userSvc.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	search := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("search")))
	if search == "" {
		writeJSON(w, http.StatusOK, allUsers)
		return
	}

	filtered := make([]users.User, 0, len(allUsers))
	for _, user := range allUsers {
		if containsSearch(search, user.Name, user.Email, user.Bio, user.Phone) {
			filtered = append(filtered, user)
		}
	}
	writeJSON(w, http.StatusOK, filtered)
}

func (h *Handler) ListOrgs(w http.ResponseWriter, r *http.Request) {
	verified, err := parseOptionalBoolParam(r.URL.Query().Get("verified"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "verified must be true or false")
		return
	}

	allOrgs, err := h.orgSvc.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	search := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("search")))
	filtered := make([]orgs.Org, 0, len(allOrgs))
	for _, org := range allOrgs {
		if verified != nil && org.Verified != *verified {
			continue
		}
		if search != "" && !containsSearch(search, org.Name, org.Email, org.Description, org.Location, org.Phone, org.Website) {
			continue
		}
		filtered = append(filtered, org)
	}
	writeJSON(w, http.StatusOK, filtered)
}

func (h *Handler) SetOrgVerification(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req setOrgVerificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Verified == nil {
		writeError(w, http.StatusBadRequest, "verified is required")
		return
	}

	org, err := h.orgSvc.Update(r.Context(), id, map[string]any{"verified": *req.Verified})
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

func (h *Handler) ListOpportunities(w http.ResponseWriter, r *http.Request) {
	opps, err := h.oppSvc.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	search := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("search")))
	category := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("category")))
	orgID := strings.TrimSpace(r.URL.Query().Get("org_id"))

	filtered := make([]opportunities.Opportunity, 0, len(opps))
	for _, opp := range opps {
		if orgID != "" && !matchesRecordID(opp.OrgID.String(), orgID, "orgs") {
			continue
		}
		if category != "" && strings.ToLower(opp.Category) != category {
			continue
		}
		if search != "" && !containsSearch(search, opp.Title, opp.Category, opp.Description, opp.Location) {
			continue
		}
		filtered = append(filtered, opp)
	}
	writeJSON(w, http.StatusOK, filtered)
}

func (h *Handler) CreateOpportunity(w http.ResponseWriter, r *http.Request) {
	var req opportunityCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	orgID := strings.TrimSpace(req.OrgID)
	if pathOrgID := strings.TrimSpace(chi.URLParam(r, "id")); pathOrgID != "" {
		if orgID != "" && !matchesRecordID("orgs:"+pathOrgID, orgID, "orgs") {
			writeError(w, http.StatusBadRequest, "body org_id must match path org id")
			return
		}
		orgID = pathOrgID
	}

	orgBareID, ok := bareRecordID(orgID, "orgs")
	if !ok {
		writeError(w, http.StatusBadRequest, "org_id is required")
		return
	}

	org, err := h.orgSvc.GetByID(r.Context(), orgBareID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if org == nil {
		writeError(w, http.StatusNotFound, "org not found")
		return
	}

	if strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Category) == "" || strings.TrimSpace(req.Description) == "" || req.Duration <= 0 || strings.TrimSpace(req.Location) == "" || req.MaxSpots <= 0 {
		writeError(w, http.StatusBadRequest, "org_id, title, category, description, positive duration, location and positive max_spots are required")
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

	opp, err := h.oppSvc.Create(r.Context(), "orgs:"+orgBareID, opportunities.Opportunity{
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

func (h *Handler) UpdateOpportunity(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req opportunityUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	updates, ok := opportunityUpdatesFromRequest(w, req)
	if !ok {
		return
	}

	opp, err := h.oppSvc.UpdateAsAdmin(r.Context(), id, updates)
	if err != nil {
		if errors.Is(err, opportunities.ErrNotFound) {
			writeError(w, http.StatusNotFound, "opportunity not found")
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

func (h *Handler) ListApplications(w http.ResponseWriter, r *http.Request) {
	status := applications.Status(strings.TrimSpace(r.URL.Query().Get("status")))
	if status != "" && !isListStatus(status) {
		writeError(w, http.StatusBadRequest, "status must be pending, accepted or rejected")
		return
	}

	apps, err := h.appSvc.ListDetailed(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	userID := strings.TrimSpace(r.URL.Query().Get("user_id"))
	oppID := strings.TrimSpace(r.URL.Query().Get("opportunity_id"))
	orgID := strings.TrimSpace(r.URL.Query().Get("org_id"))

	filtered := make([]applications.OrgApplication, 0, len(apps))
	for _, app := range apps {
		if status != "" && app.Status != status {
			continue
		}
		if userID != "" && !matchesRecordID(app.UserID.String(), userID, "users") {
			continue
		}
		if oppID != "" && !matchesRecordID(app.OpportunityID.String(), oppID, "opportunities") {
			continue
		}
		if orgID != "" && (app.Opportunity == nil || !matchesRecordID(app.Opportunity.OrgID.String(), orgID, "orgs")) {
			continue
		}
		filtered = append(filtered, app)
	}
	writeJSON(w, http.StatusOK, filtered)
}

func (h *Handler) ListOpportunityApplications(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	opp, err := h.oppSvc.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if opp == nil {
		writeError(w, http.StatusNotFound, "opportunity not found")
		return
	}

	apps, err := h.appSvc.ListDetailedByOpportunity(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, opportunityApplicationsResponse{
		Opportunity:  opp,
		Applications: apps,
	})
}

func (h *Handler) UpdateApplicationStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req updateStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	status, ok := applications.ParseDecisionStatus(req.Status)
	if !ok {
		writeError(w, http.StatusBadRequest, "status must be 'accepted' or 'rejected'")
		return
	}

	app, err := h.appSvc.UpdateStatusAsAdminDetailed(r.Context(), id, status)
	if err != nil {
		if errors.Is(err, applications.ErrNotFound) {
			writeError(w, http.StatusNotFound, "application not found")
			return
		}
		if errors.Is(err, applications.ErrNoSpotsLeft) {
			writeError(w, http.StatusConflict, "no spots left")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, app)
}

func (h *Handler) SendCampaign(w http.ResponseWriter, r *http.Request) {
	if h.mailer == nil {
		writeError(w, http.StatusInternalServerError, "mailer is not configured")
		return
	}

	var req sendCampaignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	audience := normalizeAudience(req.Audience)
	subject := strings.TrimSpace(req.Subject)
	body := strings.TrimSpace(req.Body)
	if audience == "" {
		writeError(w, http.StatusBadRequest, "audience must be users, orgs or all")
		return
	}
	if subject == "" || body == "" {
		writeError(w, http.StatusBadRequest, "subject and body are required")
		return
	}
	if strings.ContainsAny(subject, "\r\n") {
		writeError(w, http.StatusBadRequest, "subject cannot contain line breaks")
		return
	}

	recipients, err := h.campaignRecipients(r, audience)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := sendCampaignResponse{
		InvalidEmails: make([]string, 0),
	}
	seen := make(map[string]struct{}, len(recipients))
	for _, rawEmail := range recipients {
		email := strings.ToLower(strings.TrimSpace(rawEmail))
		if email == "" {
			resp.Skipped++
			continue
		}
		if _, ok := seen[email]; ok {
			resp.Skipped++
			continue
		}
		seen[email] = struct{}{}

		if _, err := netmail.ParseAddress(email); err != nil {
			resp.InvalidEmails = append(resp.InvalidEmails, email)
			resp.Skipped++
			continue
		}

		if err := h.mailer.SendCampaign(r.Context(), email, subject, body); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to send campaign emails")
			return
		}
		resp.Sent++
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) campaignRecipients(r *http.Request, audience string) ([]string, error) {
	recipients := make([]string, 0)
	if audience == "users" || audience == "all" {
		allUsers, err := h.userSvc.List(r.Context())
		if err != nil {
			return nil, err
		}
		for _, user := range allUsers {
			recipients = append(recipients, user.Email)
		}
	}
	if audience == "orgs" || audience == "all" {
		allOrgs, err := h.orgSvc.List(r.Context())
		if err != nil {
			return nil, err
		}
		for _, org := range allOrgs {
			recipients = append(recipients, org.Email)
		}
	}
	return recipients, nil
}

func opportunityUpdatesFromRequest(w http.ResponseWriter, req opportunityUpdateRequest) (map[string]any, bool) {
	updates := map[string]any{}
	if req.Title != nil {
		if strings.TrimSpace(*req.Title) == "" {
			writeError(w, http.StatusBadRequest, "title cannot be empty")
			return nil, false
		}
		updates["title"] = *req.Title
	}
	if req.Category != nil {
		if strings.TrimSpace(*req.Category) == "" {
			writeError(w, http.StatusBadRequest, "category cannot be empty")
			return nil, false
		}
		updates["category"] = *req.Category
	}
	if req.Difficulty != nil {
		updates["difficulty"] = *req.Difficulty
	}
	if req.Description != nil {
		if strings.TrimSpace(*req.Description) == "" {
			writeError(w, http.StatusBadRequest, "description cannot be empty")
			return nil, false
		}
		updates["description"] = *req.Description
	}
	if req.Date != nil {
		if req.Date.IsZero() {
			writeError(w, http.StatusBadRequest, "date is required")
			return nil, false
		}
		updates["date"] = *req.Date
	}
	if req.Duration != nil {
		if *req.Duration <= 0 {
			writeError(w, http.StatusBadRequest, "duration must be positive")
			return nil, false
		}
		updates["duration"] = *req.Duration
	}
	if req.Location != nil {
		if strings.TrimSpace(*req.Location) == "" {
			writeError(w, http.StatusBadRequest, "location cannot be empty")
			return nil, false
		}
		updates["location"] = *req.Location
	}
	if req.MaxSpots != nil {
		if *req.MaxSpots <= 0 {
			writeError(w, http.StatusBadRequest, "max_spots must be positive")
			return nil, false
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
		return nil, false
	}
	return updates, true
}

func parseOptionalBoolParam(raw string) (*bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func containsSearch(search string, values ...string) bool {
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), search) {
			return true
		}
	}
	return false
}

func matchesRecordID(actual, wanted, table string) bool {
	wanted = strings.TrimSpace(wanted)
	if wanted == "" {
		return true
	}
	if !strings.Contains(wanted, ":") {
		wanted = table + ":" + wanted
	}
	return actual == wanted
}

func bareRecordID(value, table string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	if strings.Contains(value, ":") {
		parts := strings.SplitN(value, ":", 2)
		if len(parts) != 2 || parts[0] != table || strings.TrimSpace(parts[1]) == "" {
			return "", false
		}
		return parts[1], true
	}
	return value, true
}

func isListStatus(status applications.Status) bool {
	return status == applications.StatusPending || status == applications.StatusAccepted || status == applications.StatusRejected
}

func normalizeAudience(audience string) string {
	switch strings.ToLower(strings.TrimSpace(audience)) {
	case "users":
		return "users"
	case "orgs", "organizations":
		return "orgs"
	case "all":
		return "all"
	default:
		return ""
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
