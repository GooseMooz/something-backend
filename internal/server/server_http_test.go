package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/goosemooz/something-backend/config"
	"github.com/goosemooz/something-backend/internal/auth"
	"github.com/goosemooz/something-backend/internal/db"
	"github.com/goosemooz/something-backend/internal/mail"
	"github.com/goosemooz/something-backend/internal/opportunities"
	"github.com/goosemooz/something-backend/internal/orgs"
	"github.com/goosemooz/something-backend/internal/server"
	"github.com/goosemooz/something-backend/internal/testhelper"
	"github.com/goosemooz/something-backend/internal/users"
)

var (
	testDB  *db.DB
	testCfg *config.Config
)

type capturedResetEmail struct {
	to       string
	resetURL string
}

type stubMailer struct {
	sent []capturedResetEmail
}

func (m *stubMailer) SendPasswordReset(_ context.Context, to, resetURL string) error {
	m.sent = append(m.sent, capturedResetEmail{to: to, resetURL: resetURL})
	return nil
}

func TestMain(m *testing.M) {
	database, err := testhelper.NewDB()
	if err != nil {
		if os.Getenv("CI") != "" {
			fmt.Printf("server HTTP tests require db in CI: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("skipping server HTTP tests (no db): %v\n", err)
		os.Exit(0)
	}
	testDB = database
	testCfg = &config.Config{
		JWTSecret:        "server-http-test-secret",
		AccessTokenTTL:   15 * time.Minute,
		RefreshTokenTTL:  30 * 24 * time.Hour,
		PasswordResetTTL: time.Hour,
		AppBaseURL:       "https://example.com",
	}
	os.Exit(m.Run())
}

func newRouter() http.Handler {
	return newRouterWithMailer(&stubMailer{})
}

func newRouterWithMailer(mailer mail.Mailer) http.Handler {
	r := chi.NewRouter()
	server.SetupRoutes(r, testDB, testCfg, nil, mailer)
	return r
}

func createUser(t *testing.T, email string) (*users.User, string) {
	t.Helper()
	user, err := users.NewService(testDB).Create(context.Background(), email, "hash", "User "+email)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	token, err := auth.GenerateToken(user.ID.String(), testCfg)
	if err != nil {
		t.Fatalf("generate user token: %v", err)
	}
	return user, token
}

func createOrg(t *testing.T, email string) (*orgs.Org, string) {
	t.Helper()
	org, err := orgs.NewService(testDB).Create(context.Background(), "Org "+email, "hash", email, "Vancouver")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	token, err := auth.GenerateToken(org.ID.String(), testCfg)
	if err != nil {
		t.Fatalf("generate org token: %v", err)
	}
	return org, token
}

func createOpportunity(t *testing.T, orgID string, title string) *opportunities.Opportunity {
	t.Helper()
	opp, err := opportunities.NewService(testDB).Create(context.Background(), orgID, opportunities.Opportunity{
		Title:       title,
		Category:    "environment",
		Description: "Test opportunity",
		Date:        time.Now().Add(24 * time.Hour),
		Duration:    2,
		Location:    "Vancouver",
		MaxSpots:    5,
	})
	if err != nil {
		t.Fatalf("create opportunity: %v", err)
	}
	return opp
}

func doJSONRequest(t *testing.T, h http.Handler, method, path string, body any, token string, remoteAddr string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == nil {
		req = httptest.NewRequest(method, path, nil)
	} else {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		req = httptest.NewRequest(method, path, bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if remoteAddr != "" {
		req.RemoteAddr = remoteAddr
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func doMultipartRequest(t *testing.T, h http.Handler, method, path, fieldName, filename string, content []byte, token string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile(fieldName, filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req := httptest.NewRequest(method, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func doJSONRequestWithCookies(t *testing.T, h http.Handler, method, path string, body any, token string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == nil {
		req = httptest.NewRequest(method, path, nil)
	} else {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		req = httptest.NewRequest(method, path, bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func cookieByName(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

func TestApplyRejectsOrgTokens(t *testing.T) {
	router := newRouter()
	org, orgToken := createOrg(t, "apply-org@example.com")
	opp := createOpportunity(t, org.ID.String(), "Org-only Apply Rejection")

	rec := doJSONRequest(t, router, http.MethodPost, "/opportunities/"+opp.ID.ID.(string)+"/apply", map[string]any{}, orgToken, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "users only") {
		t.Fatalf("expected users only error, got %s", rec.Body.String())
	}
}

func TestApplicationsRoutesEnforceRoles(t *testing.T) {
	router := newRouter()
	user, userToken := createUser(t, "apps-user@example.com")
	org, orgToken := createOrg(t, "apps-org@example.com")
	updatedUser, err := users.NewService(testDB).Update(context.Background(), user.ID.ID.(string), map[string]any{
		"s3_pdf": "https://example.com/resume.pdf",
	})
	if err != nil || updatedUser == nil {
		t.Fatalf("expected user resume update to succeed, got %v", err)
	}
	opp := createOpportunity(t, org.ID.String(), "Role Separation")
	applyRec := doJSONRequest(t, router, http.MethodPost, "/opportunities/"+opp.ID.ID.(string)+"/apply", map[string]any{}, userToken, "")
	if applyRec.Code != http.StatusCreated {
		t.Fatalf("expected application creation, got %d: %s", applyRec.Code, applyRec.Body.String())
	}

	orgListRec := doJSONRequest(t, router, http.MethodGet, "/applications", nil, orgToken, "")
	if orgListRec.Code != http.StatusForbidden {
		t.Fatalf("expected org list to be forbidden, got %d", orgListRec.Code)
	}

	var listResp []map[string]any
	userListRec := doJSONRequest(t, router, http.MethodGet, "/applications", nil, userToken, "")
	if userListRec.Code != http.StatusOK {
		t.Fatalf("expected user list to succeed, got %d: %s", userListRec.Code, userListRec.Body.String())
	}
	if err := json.Unmarshal(userListRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal list response: %v", err)
	}
	if len(listResp) == 0 {
		t.Fatal("expected at least one application")
	}

	appID := strings.TrimPrefix(listResp[0]["id"].(string), "applications:")
	userUpdateRec := doJSONRequest(t, router, http.MethodPut, "/applications/"+appID, map[string]string{"status": "accepted"}, userToken, "")
	if userUpdateRec.Code != http.StatusForbidden {
		t.Fatalf("expected user status update to be forbidden, got %d", userUpdateRec.Code)
	}

	orgAppsRec := doJSONRequest(t, router, http.MethodGet, "/orgs/"+org.ID.ID.(string)+"/applications", nil, orgToken, "")
	if orgAppsRec.Code != http.StatusOK {
		t.Fatalf("expected org applications to succeed, got %d: %s", orgAppsRec.Code, orgAppsRec.Body.String())
	}
	if !strings.Contains(orgAppsRec.Body.String(), "\"s3_pdf\":\"https://example.com/resume.pdf\"") {
		t.Fatalf("expected org applications payload to include applicant resume, got %s", orgAppsRec.Body.String())
	}
}

func TestApplyMissingOpportunityReturnsNotFound(t *testing.T) {
	router := newRouter()
	_, token := createUser(t, "missing-opp-user@example.com")

	rec := doJSONRequest(t, router, http.MethodPost, "/opportunities/does-not-exist/apply", map[string]any{}, token, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDuplicateRegisterReturnsConflict(t *testing.T) {
	router := newRouter()
	email := "duplicate-user@example.com"
	_, _ = createUser(t, email)

	rec := doJSONRequest(t, router, http.MethodPost, "/auth/register", map[string]string{
		"name":     "Duplicate User",
		"email":    email,
		"password": "secret123",
	}, "", "198.51.100.10:1234")
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUserUpdateCanClearBio(t *testing.T) {
	router := newRouter()
	user, token := createUser(t, "clear-bio@example.com")
	id := user.ID.ID.(string)

	first := doJSONRequest(t, router, http.MethodPut, "/users/"+id, map[string]any{"bio": "has content"}, token, "")
	if first.Code != http.StatusOK {
		t.Fatalf("expected initial update to succeed, got %d: %s", first.Code, first.Body.String())
	}

	second := doJSONRequest(t, router, http.MethodPut, "/users/"+id, map[string]any{"bio": ""}, token, "")
	if second.Code != http.StatusOK {
		t.Fatalf("expected clearing bio to succeed, got %d: %s", second.Code, second.Body.String())
	}

	fresh, err := users.NewService(testDB).GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if fresh.Bio != "" {
		t.Fatalf("expected bio to be cleared, got %q", fresh.Bio)
	}
}

func TestResumeUploadRejectsNonPDF(t *testing.T) {
	router := newRouter()
	user, token := createUser(t, "resume-check@example.com")
	id := user.ID.ID.(string)

	rec := doMultipartRequest(t, router, http.MethodPost, "/users/"+id+"/resume", "file", "not-a-pdf.txt", []byte("hello world"), token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "file must be a PDF") {
		t.Fatalf("unexpected response body: %s", rec.Body.String())
	}
}

func TestPFPUploadRejectsNonAllowedImageType(t *testing.T) {
	router := newRouter()
	user, token := createUser(t, "pfp-check@example.com")
	id := user.ID.ID.(string)

	rec := doMultipartRequest(t, router, http.MethodPost, "/users/"+id+"/pfp", "file", "not-an-image.pdf", []byte("%PDF-1.4 fake"), token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "JPG, PNG, GIF, or WebP") {
		t.Fatalf("unexpected response body: %s", rec.Body.String())
	}
}

func TestOpportunityCreateRejectsInvalidValues(t *testing.T) {
	router := newRouter()
	_, token := createOrg(t, "invalid-opp-org@example.com")

	rec := doJSONRequest(t, router, http.MethodPost, "/opportunities", map[string]any{
		"title":       "Broken Opportunity",
		"category":    "environment",
		"description": "bad inputs",
		"date":        time.Now().Add(-time.Hour).Format(time.RFC3339),
		"duration":    -1,
		"location":    "Vancouver",
		"max_spots":   -1,
	}, token, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestOpportunityUpdateForOrg(t *testing.T) {
	router := newRouter()
	org, token := createOrg(t, "opp-update-org@example.com")
	opp := createOpportunity(t, org.ID.String(), "Needs Update")

	rec := doJSONRequest(t, router, http.MethodPut, "/opportunities/"+opp.ID.ID.(string), map[string]any{
		"title":     "Updated Opportunity",
		"duration":  3.5,
		"max_spots": 9,
	}, token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "\"title\":\"Updated Opportunity\"") {
		t.Fatalf("unexpected response body: %s", rec.Body.String())
	}
}

func TestApplicationStatusUpdateReturnsUpdatedOpportunitySpots(t *testing.T) {
	router := newRouter()
	_, userToken := createUser(t, "spots-user@example.com")
	org, orgToken := createOrg(t, "spots-org@example.com")
	opp := createOpportunity(t, org.ID.String(), "Spot Tracking")

	applyRec := doJSONRequest(t, router, http.MethodPost, "/opportunities/"+opp.ID.ID.(string)+"/apply", map[string]any{}, userToken, "")
	if applyRec.Code != http.StatusCreated {
		t.Fatalf("expected application creation, got %d: %s", applyRec.Code, applyRec.Body.String())
	}

	orgAppsRec := doJSONRequest(t, router, http.MethodGet, "/orgs/"+org.ID.ID.(string)+"/applications", nil, orgToken, "")
	if orgAppsRec.Code != http.StatusOK {
		t.Fatalf("expected org applications to succeed, got %d: %s", orgAppsRec.Code, orgAppsRec.Body.String())
	}

	var apps []map[string]any
	if err := json.Unmarshal(orgAppsRec.Body.Bytes(), &apps); err != nil {
		t.Fatalf("unmarshal applications response: %v", err)
	}
	if len(apps) == 0 {
		t.Fatal("expected at least one application")
	}

	appID := strings.TrimPrefix(apps[0]["id"].(string), "applications:")
	updateRec := doJSONRequest(t, router, http.MethodPut, "/applications/"+appID, map[string]string{"status": "accepted"}, orgToken, "")
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected status update to succeed, got %d: %s", updateRec.Code, updateRec.Body.String())
	}
	if !strings.Contains(updateRec.Body.String(), "\"spots_left\":4") {
		t.Fatalf("expected updated opportunity spots_left in response, got %s", updateRec.Body.String())
	}
}

func TestAuthLoginRateLimit(t *testing.T) {
	router := newRouter()
	_, _ = createUser(t, "rate-limit-user@example.com")

	var rec *httptest.ResponseRecorder
	for i := 0; i < 11; i++ {
		rec = doJSONRequest(t, router, http.MethodPost, "/auth/login", map[string]string{
			"email":    "rate-limit-user@example.com",
			"password": "wrong-password",
		}, "", "203.0.113.50:4000")
	}
	if rec == nil {
		t.Fatal("expected a response recorder")
	}
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header to be set")
	}
}

func TestLoginRefreshAndLogoutFlow(t *testing.T) {
	router := newRouter()
	email := fmt.Sprintf("refresh-user-%d@example.com", time.Now().UnixNano())
	password := "secret123"

	registerRec := doJSONRequest(t, router, http.MethodPost, "/auth/register", map[string]string{
		"name":     "Refresh User",
		"email":    email,
		"password": password,
	}, "", "")
	if registerRec.Code != http.StatusCreated {
		t.Fatalf("expected register to succeed, got %d: %s", registerRec.Code, registerRec.Body.String())
	}

	loginRec := doJSONRequest(t, router, http.MethodPost, "/auth/login", map[string]string{
		"email":    email,
		"password": password,
	}, "", "")
	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected login to succeed, got %d: %s", loginRec.Code, loginRec.Body.String())
	}

	var loginBody map[string]string
	if err := json.Unmarshal(loginRec.Body.Bytes(), &loginBody); err != nil {
		t.Fatalf("unmarshal login body: %v", err)
	}
	if loginBody["token"] == "" || loginBody["access_token"] == "" {
		t.Fatalf("expected login response to include tokens, got %v", loginBody)
	}
	refreshCookie := cookieByName(loginRec.Result().Cookies(), "refresh_token")
	if refreshCookie == nil || refreshCookie.Value == "" {
		t.Fatal("expected refresh token cookie on login")
	}

	refreshRec := doJSONRequestWithCookies(t, router, http.MethodPost, "/auth/refresh", nil, "", []*http.Cookie{refreshCookie})
	if refreshRec.Code != http.StatusOK {
		t.Fatalf("expected refresh to succeed, got %d: %s", refreshRec.Code, refreshRec.Body.String())
	}

	var refreshBody map[string]string
	if err := json.Unmarshal(refreshRec.Body.Bytes(), &refreshBody); err != nil {
		t.Fatalf("unmarshal refresh body: %v", err)
	}
	if refreshBody["access_token"] == "" {
		t.Fatal("expected refreshed access token")
	}
	if refreshBody["access_token"] == loginBody["access_token"] {
		t.Fatal("expected rotated access token to differ from login token")
	}
	rotatedCookie := cookieByName(refreshRec.Result().Cookies(), "refresh_token")
	if rotatedCookie == nil || rotatedCookie.Value == "" || rotatedCookie.Value == refreshCookie.Value {
		t.Fatal("expected rotated refresh token cookie")
	}

	reuseRec := doJSONRequestWithCookies(t, router, http.MethodPost, "/auth/refresh", nil, "", []*http.Cookie{refreshCookie})
	if reuseRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected reused refresh token to fail, got %d: %s", reuseRec.Code, reuseRec.Body.String())
	}

	logoutRec := doJSONRequestWithCookies(t, router, http.MethodPost, "/auth/logout", nil, "", []*http.Cookie{rotatedCookie})
	if logoutRec.Code != http.StatusOK {
		t.Fatalf("expected logout to succeed, got %d: %s", logoutRec.Code, logoutRec.Body.String())
	}
	cleared := cookieByName(logoutRec.Result().Cookies(), "refresh_token")
	if cleared == nil || cleared.MaxAge != -1 {
		t.Fatal("expected logout to clear refresh token cookie")
	}
}

func TestUserChangePasswordRevokesRefreshTokens(t *testing.T) {
	router := newRouter()
	email := fmt.Sprintf("change-password-user-%d@example.com", time.Now().UnixNano())
	oldPassword := "secret123"
	newPassword := "newsecret456"

	registerRec := doJSONRequest(t, router, http.MethodPost, "/auth/register", map[string]string{
		"name":     "Change Password User",
		"email":    email,
		"password": oldPassword,
	}, "", "")
	if registerRec.Code != http.StatusCreated {
		t.Fatalf("expected register to succeed, got %d: %s", registerRec.Code, registerRec.Body.String())
	}

	loginRec := doJSONRequest(t, router, http.MethodPost, "/auth/login", map[string]string{
		"email":    email,
		"password": oldPassword,
	}, "", "")
	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected login to succeed, got %d: %s", loginRec.Code, loginRec.Body.String())
	}

	var loginBody map[string]string
	if err := json.Unmarshal(loginRec.Body.Bytes(), &loginBody); err != nil {
		t.Fatalf("unmarshal login body: %v", err)
	}
	token := loginBody["access_token"]
	if token == "" {
		t.Fatal("expected access token on login")
	}

	var createdUser users.User
	if err := json.Unmarshal(registerRec.Body.Bytes(), &createdUser); err != nil {
		t.Fatalf("unmarshal register body: %v", err)
	}
	userID := createdUser.ID.ID.(string)

	refreshCookie := cookieByName(loginRec.Result().Cookies(), "refresh_token")
	if refreshCookie == nil || refreshCookie.Value == "" {
		t.Fatal("expected refresh token cookie on login")
	}

	changeRec := doJSONRequestWithCookies(t, router, http.MethodPut, "/users/"+userID+"/password", map[string]string{
		"current_password": oldPassword,
		"new_password":     newPassword,
	}, token, []*http.Cookie{refreshCookie})
	if changeRec.Code != http.StatusOK {
		t.Fatalf("expected password change to succeed, got %d: %s", changeRec.Code, changeRec.Body.String())
	}
	cleared := cookieByName(changeRec.Result().Cookies(), "refresh_token")
	if cleared == nil || cleared.MaxAge != -1 {
		t.Fatal("expected password change to clear refresh token cookie")
	}

	reuseRefreshRec := doJSONRequestWithCookies(t, router, http.MethodPost, "/auth/refresh", nil, "", []*http.Cookie{refreshCookie})
	if reuseRefreshRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected revoked refresh token to fail, got %d: %s", reuseRefreshRec.Code, reuseRefreshRec.Body.String())
	}

	oldLoginRec := doJSONRequest(t, router, http.MethodPost, "/auth/login", map[string]string{
		"email":    email,
		"password": oldPassword,
	}, "", "")
	if oldLoginRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected old password login to fail, got %d: %s", oldLoginRec.Code, oldLoginRec.Body.String())
	}

	newLoginRec := doJSONRequest(t, router, http.MethodPost, "/auth/login", map[string]string{
		"email":    email,
		"password": newPassword,
	}, "", "")
	if newLoginRec.Code != http.StatusOK {
		t.Fatalf("expected new password login to succeed, got %d: %s", newLoginRec.Code, newLoginRec.Body.String())
	}
}

func TestOrgChangePasswordRevokesRefreshTokens(t *testing.T) {
	router := newRouter()
	email := fmt.Sprintf("change-password-org-%d@example.com", time.Now().UnixNano())
	oldPassword := "secret123"
	newPassword := "newsecret456"

	registerRec := doJSONRequest(t, router, http.MethodPost, "/auth/org/register", map[string]string{
		"name":     "Change Password Org",
		"email":    email,
		"password": oldPassword,
		"location": "Vancouver",
	}, "", "")
	if registerRec.Code != http.StatusCreated {
		t.Fatalf("expected org register to succeed, got %d: %s", registerRec.Code, registerRec.Body.String())
	}

	loginRec := doJSONRequest(t, router, http.MethodPost, "/auth/org/login", map[string]string{
		"email":    email,
		"password": oldPassword,
	}, "", "")
	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected org login to succeed, got %d: %s", loginRec.Code, loginRec.Body.String())
	}

	var loginBody map[string]string
	if err := json.Unmarshal(loginRec.Body.Bytes(), &loginBody); err != nil {
		t.Fatalf("unmarshal org login body: %v", err)
	}
	token := loginBody["access_token"]
	if token == "" {
		t.Fatal("expected access token on org login")
	}

	var createdOrg orgs.Org
	if err := json.Unmarshal(registerRec.Body.Bytes(), &createdOrg); err != nil {
		t.Fatalf("unmarshal org register body: %v", err)
	}
	orgID := createdOrg.ID.ID.(string)

	refreshCookie := cookieByName(loginRec.Result().Cookies(), "refresh_token")
	if refreshCookie == nil || refreshCookie.Value == "" {
		t.Fatal("expected refresh token cookie on org login")
	}

	changeRec := doJSONRequestWithCookies(t, router, http.MethodPut, "/orgs/"+orgID+"/password", map[string]string{
		"current_password": oldPassword,
		"new_password":     newPassword,
	}, token, []*http.Cookie{refreshCookie})
	if changeRec.Code != http.StatusOK {
		t.Fatalf("expected org password change to succeed, got %d: %s", changeRec.Code, changeRec.Body.String())
	}
	cleared := cookieByName(changeRec.Result().Cookies(), "refresh_token")
	if cleared == nil || cleared.MaxAge != -1 {
		t.Fatal("expected org password change to clear refresh token cookie")
	}

	reuseRefreshRec := doJSONRequestWithCookies(t, router, http.MethodPost, "/auth/refresh", nil, "", []*http.Cookie{refreshCookie})
	if reuseRefreshRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected revoked org refresh token to fail, got %d: %s", reuseRefreshRec.Code, reuseRefreshRec.Body.String())
	}

	oldLoginRec := doJSONRequest(t, router, http.MethodPost, "/auth/org/login", map[string]string{
		"email":    email,
		"password": oldPassword,
	}, "", "")
	if oldLoginRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected old org password login to fail, got %d: %s", oldLoginRec.Code, oldLoginRec.Body.String())
	}

	newLoginRec := doJSONRequest(t, router, http.MethodPost, "/auth/org/login", map[string]string{
		"email":    email,
		"password": newPassword,
	}, "", "")
	if newLoginRec.Code != http.StatusOK {
		t.Fatalf("expected new org password login to succeed, got %d: %s", newLoginRec.Code, newLoginRec.Body.String())
	}
}

func TestForgotAndResetPasswordFlow(t *testing.T) {
	testMailer := &stubMailer{}
	router := newRouterWithMailer(testMailer)
	email := fmt.Sprintf("forgot-password-user-%d@example.com", time.Now().UnixNano())
	oldPassword := "secret123"
	newPassword := "newsecret456"

	registerRec := doJSONRequest(t, router, http.MethodPost, "/auth/register", map[string]string{
		"name":     "Forgot Password User",
		"email":    email,
		"password": oldPassword,
	}, "", "")
	if registerRec.Code != http.StatusCreated {
		t.Fatalf("expected register to succeed, got %d: %s", registerRec.Code, registerRec.Body.String())
	}

	loginRec := doJSONRequest(t, router, http.MethodPost, "/auth/login", map[string]string{
		"email":    email,
		"password": oldPassword,
	}, "", "")
	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected initial login to succeed, got %d: %s", loginRec.Code, loginRec.Body.String())
	}
	refreshCookie := cookieByName(loginRec.Result().Cookies(), "refresh_token")
	if refreshCookie == nil || refreshCookie.Value == "" {
		t.Fatal("expected refresh token cookie on login")
	}

	forgotRec := doJSONRequest(t, router, http.MethodPost, "/auth/forgot-password", map[string]string{
		"email": email,
	}, "", "")
	if forgotRec.Code != http.StatusOK {
		t.Fatalf("expected forgot password to succeed, got %d: %s", forgotRec.Code, forgotRec.Body.String())
	}
	if len(testMailer.sent) != 1 {
		t.Fatalf("expected one password reset email, got %d", len(testMailer.sent))
	}

	resetURL := testMailer.sent[0].resetURL
	token := ""
	if idx := strings.Index(resetURL, "token="); idx != -1 {
		token = resetURL[idx+len("token="):]
	}
	if token == "" {
		t.Fatalf("expected reset token in reset URL, got %s", resetURL)
	}

	resetRec := doJSONRequestWithCookies(t, router, http.MethodPost, "/auth/reset-password", map[string]string{
		"token":        token,
		"new_password": newPassword,
	}, "", []*http.Cookie{refreshCookie})
	if resetRec.Code != http.StatusOK {
		t.Fatalf("expected reset password to succeed, got %d: %s", resetRec.Code, resetRec.Body.String())
	}
	cleared := cookieByName(resetRec.Result().Cookies(), "refresh_token")
	if cleared == nil || cleared.MaxAge != -1 {
		t.Fatal("expected reset password to clear refresh token cookie")
	}

	reuseRec := doJSONRequest(t, router, http.MethodPost, "/auth/reset-password", map[string]string{
		"token":        token,
		"new_password": "anotherpass123",
	}, "", "")
	if reuseRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected reused reset token to fail, got %d: %s", reuseRec.Code, reuseRec.Body.String())
	}

	oldLoginRec := doJSONRequest(t, router, http.MethodPost, "/auth/login", map[string]string{
		"email":    email,
		"password": oldPassword,
	}, "", "")
	if oldLoginRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected old password login to fail, got %d: %s", oldLoginRec.Code, oldLoginRec.Body.String())
	}

	newLoginRec := doJSONRequest(t, router, http.MethodPost, "/auth/login", map[string]string{
		"email":    email,
		"password": newPassword,
	}, "", "")
	if newLoginRec.Code != http.StatusOK {
		t.Fatalf("expected new password login to succeed, got %d: %s", newLoginRec.Code, newLoginRec.Body.String())
	}

	reuseRefreshRec := doJSONRequestWithCookies(t, router, http.MethodPost, "/auth/refresh", nil, "", []*http.Cookie{refreshCookie})
	if reuseRefreshRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected old refresh token to be revoked, got %d: %s", reuseRefreshRec.Code, reuseRefreshRec.Body.String())
	}
}

func TestForgotAndResetPasswordFlowForOrg(t *testing.T) {
	testMailer := &stubMailer{}
	router := newRouterWithMailer(testMailer)
	email := fmt.Sprintf("forgot-password-org-%d@example.com", time.Now().UnixNano())
	oldPassword := "secret123"
	newPassword := "newsecret456"

	registerRec := doJSONRequest(t, router, http.MethodPost, "/auth/org/register", map[string]string{
		"name":     "Forgot Password Org",
		"email":    email,
		"password": oldPassword,
		"location": "Vancouver",
	}, "", "")
	if registerRec.Code != http.StatusCreated {
		t.Fatalf("expected org register to succeed, got %d: %s", registerRec.Code, registerRec.Body.String())
	}

	loginRec := doJSONRequest(t, router, http.MethodPost, "/auth/org/login", map[string]string{
		"email":    email,
		"password": oldPassword,
	}, "", "")
	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected initial org login to succeed, got %d: %s", loginRec.Code, loginRec.Body.String())
	}
	refreshCookie := cookieByName(loginRec.Result().Cookies(), "refresh_token")
	if refreshCookie == nil || refreshCookie.Value == "" {
		t.Fatal("expected refresh token cookie on org login")
	}

	forgotRec := doJSONRequest(t, router, http.MethodPost, "/auth/forgot-password", map[string]string{
		"email": email,
	}, "", "")
	if forgotRec.Code != http.StatusOK {
		t.Fatalf("expected forgot password to succeed, got %d: %s", forgotRec.Code, forgotRec.Body.String())
	}
	if len(testMailer.sent) != 1 {
		t.Fatalf("expected one password reset email, got %d", len(testMailer.sent))
	}

	resetURL := testMailer.sent[0].resetURL
	token := ""
	if idx := strings.Index(resetURL, "token="); idx != -1 {
		token = resetURL[idx+len("token="):]
	}
	if token == "" {
		t.Fatalf("expected reset token in reset URL, got %s", resetURL)
	}

	resetRec := doJSONRequestWithCookies(t, router, http.MethodPost, "/auth/reset-password", map[string]string{
		"token":        token,
		"new_password": newPassword,
	}, "", []*http.Cookie{refreshCookie})
	if resetRec.Code != http.StatusOK {
		t.Fatalf("expected reset password to succeed, got %d: %s", resetRec.Code, resetRec.Body.String())
	}

	oldLoginRec := doJSONRequest(t, router, http.MethodPost, "/auth/org/login", map[string]string{
		"email":    email,
		"password": oldPassword,
	}, "", "")
	if oldLoginRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected old org password login to fail, got %d: %s", oldLoginRec.Code, oldLoginRec.Body.String())
	}

	newLoginRec := doJSONRequest(t, router, http.MethodPost, "/auth/org/login", map[string]string{
		"email":    email,
		"password": newPassword,
	}, "", "")
	if newLoginRec.Code != http.StatusOK {
		t.Fatalf("expected new org password login to succeed, got %d: %s", newLoginRec.Code, newLoginRec.Body.String())
	}

	var loginBody map[string]string
	if err := json.Unmarshal(newLoginRec.Body.Bytes(), &loginBody); err != nil {
		t.Fatalf("unmarshal org login body: %v", err)
	}
	tokenAfterReset := loginBody["access_token"]
	if tokenAfterReset == "" {
		t.Fatal("expected access token on org login after reset")
	}
	claims, err := auth.ValidateToken(tokenAfterReset, testCfg)
	if err != nil {
		t.Fatalf("validate org token after reset: %v", err)
	}
	if !strings.HasPrefix(claims.UserID, "orgs:") {
		t.Fatalf("expected org token after reset, got %q", claims.UserID)
	}
}
