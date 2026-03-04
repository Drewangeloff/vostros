package router

import (
	"embed"
	"io/fs"
	"log"
	"net/http"

	"github.com/drewangeloff/vostros/internal/auth"
	"github.com/drewangeloff/vostros/internal/handler"
	"github.com/drewangeloff/vostros/internal/ratelimit"
)

func New(h *handler.Handler, staticFS embed.FS, authMW *auth.Middleware, limiter *ratelimit.Limiter) http.Handler {
	mux := http.NewServeMux()

	// requireAuth wraps a handler with RequireAuth middleware (defense-in-depth)
	requireAuth := func(fn http.HandlerFunc) http.Handler {
		return authMW.RequireAuth(http.HandlerFunc(fn))
	}

	// Static files
	staticSub, err := fs.Sub(staticFS, "web/static")
	if err != nil {
		log.Fatalf("static fs: %v", err)
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	// Health
	mux.HandleFunc("GET /health", h.Healthz)
	mux.HandleFunc("GET /readyz", h.Healthz)

	// Auth pages (unauthenticated)
	mux.HandleFunc("GET /login", h.ShowLogin)
	mux.HandleFunc("POST /login", h.Login)
	mux.HandleFunc("GET /register", h.ShowRegister)
	mux.HandleFunc("POST /register", h.Register)
	mux.HandleFunc("POST /logout", h.Logout)

	// Settings (require auth)
	mux.Handle("GET /settings", requireAuth(h.ShowSettings))
	mux.Handle("POST /settings", requireAuth(h.UpdateSettings))

	// Developers / API tokens (require auth)
	mux.Handle("GET /developers", requireAuth(h.ShowAPI))
	mux.Handle("POST /developers/tokens", requireAuth(h.CreateAPIToken))
	mux.Handle("DELETE /developers/tokens/{id}", requireAuth(h.DeleteAPIToken))

	// Web routes (public reads)
	mux.HandleFunc("GET /{$}", h.Home)
	mux.HandleFunc("GET /global", h.Global)
	mux.HandleFunc("GET /timeline", h.Timeline)
	mux.HandleFunc("GET /search", h.Search)
	mux.HandleFunc("GET /u/{username}", h.Profile)

	// Post actions (require auth)
	mux.Handle("POST /post", requireAuth(h.CreatePost))
	mux.Handle("DELETE /post/{id}", requireAuth(h.DeletePost))

	// Follow actions (require auth)
	mux.Handle("POST /follow/{username}", requireAuth(h.Follow))
	mux.Handle("DELETE /follow/{username}", requireAuth(h.Unfollow))

	// HTMX partials
	mux.HandleFunc("GET /htmx/timeline", h.HTMXTimeline)
	mux.HandleFunc("GET /htmx/global", h.HTMXGlobal)
	mux.HandleFunc("GET /htmx/search", h.HTMXSearch)
	mux.HandleFunc("GET /htmx/u/{username}/posts", h.HTMXUserPosts)

	// JSON API - Auth (unauthenticated)
	mux.HandleFunc("POST /api/v1/auth/register", h.Register)
	mux.HandleFunc("POST /api/v1/auth/login", h.Login)
	mux.HandleFunc("POST /api/v1/auth/refresh", h.RefreshToken)
	mux.HandleFunc("DELETE /api/v1/auth/logout", h.Logout)

	// JSON API - Timelines (public reads)
	mux.HandleFunc("GET /api/v1/timeline", h.Timeline)
	mux.HandleFunc("GET /api/v1/global", h.Global)

	// JSON API - Posts (mutations require auth)
	mux.Handle("POST /api/v1/posts", requireAuth(h.CreatePost))
	mux.HandleFunc("GET /api/v1/posts/{id}", h.GetPost)
	mux.Handle("DELETE /api/v1/posts/{id}", requireAuth(h.DeletePost))

	// JSON API - Users
	mux.HandleFunc("GET /api/v1/users/{username}", h.Profile)
	mux.Handle("POST /api/v1/users/{username}/follow", requireAuth(h.Follow))
	mux.Handle("DELETE /api/v1/users/{username}/follow", requireAuth(h.Unfollow))

	// JSON API - Search
	mux.HandleFunc("GET /api/v1/search", h.Search)

	// Wrap everything with optional auth + rate limiting + security headers + logging
	return logMiddleware(securityHeaders(limiter.Middleware(authMW.OptionalAuth(mux))))
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; connect-src 'self'")
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
