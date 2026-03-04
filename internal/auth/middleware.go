package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/drewangeloff/vostros/internal/ctxutil"
	"github.com/drewangeloff/vostros/internal/repository"
)

type Middleware struct {
	auth *Service
	repo repository.Repository
}

func NewMiddleware(auth *Service, repo repository.Repository) *Middleware {
	return &Middleware{auth: auth, repo: repo}
}

// OptionalAuth extracts user from JWT if present, but doesn't require it.
func (m *Middleware) OptionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenStr := extractToken(r)
		if tokenStr == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Check if this is an API token (vst_ prefix) rather than a JWT
		if strings.HasPrefix(tokenStr, "vst_") {
			hash := sha256.Sum256([]byte(tokenStr))
			hashHex := hex.EncodeToString(hash[:])
			apiToken, err := m.repo.GetAPITokenByHash(r.Context(), hashHex)
			if err != nil || apiToken == nil {
				next.ServeHTTP(w, r)
				return
			}
			user, err := m.repo.GetUserByID(r.Context(), apiToken.UserID)
			if err != nil || user == nil {
				next.ServeHTTP(w, r)
				return
			}
			// Touch last_used in background (best-effort, detached context)
			go m.repo.TouchAPIToken(context.Background(), apiToken.ID)
			ctx := ctxutil.SetUser(r.Context(), user)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		claims, err := m.auth.ValidateToken(tokenStr)
		if err != nil {
			// Token invalid/expired — continue without user
			next.ServeHTTP(w, r)
			return
		}

		user, err := m.repo.GetUserByID(r.Context(), claims.UserID)
		if err != nil || user == nil {
			next.ServeHTTP(w, r)
			return
		}

		// Sliding window: if cookie-based token expires in < 5 min, auto-refresh
		if _, cookieErr := r.Cookie("access_token"); cookieErr == nil {
			if claims.ExpiresAt != nil && time.Until(claims.ExpiresAt.Time) < 5*time.Minute {
				if newToken, err := m.auth.CreateAccessToken(user.ID, user.Username, user.Role); err == nil {
					http.SetCookie(w, &http.Cookie{
						Name:     "access_token",
						Value:    newToken,
						Path:     "/",
						MaxAge:   900,
						HttpOnly: true,
						Secure:   true,
						SameSite: http.SameSiteLaxMode,
					})
				}
			}
		}

		ctx := ctxutil.SetUser(r.Context(), user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAuth rejects requests without a valid JWT.
func (m *Middleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := ctxutil.GetUser(r.Context())
		if user == nil {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			} else {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
			}
			return
		}
		next.ServeHTTP(w, r)
	})
}

func extractToken(r *http.Request) string {
	// Check Authorization header first (API clients)
	if auth := r.Header.Get("Authorization"); auth != "" {
		if strings.HasPrefix(auth, "Bearer ") {
			return strings.TrimPrefix(auth, "Bearer ")
		}
	}
	// Check cookie (web clients)
	if cookie, err := r.Cookie("access_token"); err == nil {
		return cookie.Value
	}
	return ""
}
