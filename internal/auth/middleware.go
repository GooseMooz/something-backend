package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/goosemooz/something-backend/config"
)

type contextKey string

const claimsKey contextKey = "claims"

func RequireAuth(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if t == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
				return
			}

			claims, err := ValidateToken(t, cfg)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
				return
			}

			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetClaims(r *http.Request) *Claims {
	claims, _ := r.Context().Value(claimsKey).(*Claims)
	return claims
}

// TryGetClaims attempts to validate the Bearer token if present, returning nil
// on missing or invalid token. Use this on public routes where auth is optional.
func TryGetClaims(r *http.Request, cfg *config.Config) *Claims {
	t := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if t == "" {
		return nil
	}
	claims, err := ValidateToken(t, cfg)
	if err != nil {
		return nil
	}
	return claims
}

// RequireOrgAuth must be chained after RequireAuth. It rejects requests
// whose token belongs to a user account rather than an org account.
func RequireOrgAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := GetClaims(r)
		if claims == nil || !strings.HasPrefix(claims.UserID, "orgs:") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"orgs only"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireUserAuth must be chained after RequireAuth. It rejects requests
// whose token belongs to an org account rather than a user account.
func RequireUserAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := GetClaims(r)
		if claims == nil || !strings.HasPrefix(claims.UserID, "users:") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"users only"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}
