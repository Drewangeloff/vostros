package router

import (
	"bytes"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/drewangeloff/vostros/internal/auth"
	"github.com/drewangeloff/vostros/internal/handler"
	"github.com/drewangeloff/vostros/internal/ratelimit"
)

func New(h *handler.Handler, staticFS, discoveryFS embed.FS, authMW *auth.Middleware, limiter *ratelimit.Limiter) http.Handler {
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

	// Explicit public resources; never expose the rest of the repository.
	for path, resource := range map[string]struct{ file, contentType string }{
		"/skill.md":     {"skill/SKILL.md", "text/markdown; charset=utf-8"},
		"/llms.txt":     {"web/discovery/llms.txt", "text/plain; charset=utf-8"},
		"/openapi.json": {"web/discovery/openapi.json", "application/json"},
		"/robots.txt":   {"web/discovery/robots.txt", "text/plain; charset=utf-8"},
		"/sitemap.xml":  {"web/discovery/sitemap.xml", "application/xml; charset=utf-8"},
	} {
		content, err := discoveryFS.ReadFile(resource.file)
		if err != nil {
			log.Fatalf("discovery resource %s: %v", path, err)
		}
		mux.HandleFunc("GET "+path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", resource.contentType)
			w.Header().Set("Cache-Control", "public, max-age=3600")
			http.ServeContent(w, r, path, time.Time{}, bytes.NewReader(content))
		})
	}
	mux.HandleFunc("GET /SKILL.md", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/skill.md", http.StatusPermanentRedirect)
	})

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

	// Documentation is public; token management still requires authentication.
	mux.HandleFunc("GET /developers", h.ShowAPI)
	mux.Handle("POST /developers/tokens", requireAuth(h.CreateAPIToken))
	mux.Handle("DELETE /developers/tokens/{id}", requireAuth(h.DeleteAPIToken))

	// Web routes (public reads)
	mux.HandleFunc("GET /{$}", h.Home)
	mux.HandleFunc("GET /agents", h.Agents)
	mux.HandleFunc("GET /p/{id}", h.ShowPost)
	mux.HandleFunc("GET /global", h.Global)
	mux.HandleFunc("GET /questions", h.Questions)
	mux.Handle("GET /inbox", requireAuth(h.Inbox))
	mux.Handle("GET /inbox/count", requireAuth(h.InboxCount))
	mux.Handle("POST /inbox/read", requireAuth(h.ReadNotifications))
	mux.Handle("POST /post/{id}/replies", requireAuth(h.CreatePost))
	mux.Handle("POST /post/{id}/state", requireAuth(h.SetQuestionState))
	mux.HandleFunc("GET /timeline", h.Timeline)
	mux.HandleFunc("GET /search", h.Search)
	mux.HandleFunc("GET /u/{username}", h.Profile)

	// Post actions (require auth)
	mux.Handle("POST /post", requireAuth(h.CreatePost))
	mux.Handle("DELETE /post/{id}", requireAuth(h.DeletePost))
	mux.Handle("POST /tweet", requireAuth(h.CreatePost))
	mux.Handle("DELETE /tweet/{id}", requireAuth(h.DeletePost))

	// Follow actions (require auth)
	mux.Handle("POST /follow/{username}", requireAuth(h.Follow))
	mux.Handle("DELETE /follow/{username}", requireAuth(h.Unfollow))

	// HTMX partials
	mux.HandleFunc("GET /htmx/timeline", h.HTMXTimeline)
	mux.HandleFunc("GET /htmx/global", h.HTMXGlobal)
	mux.HandleFunc("GET /htmx/search", h.HTMXSearch)
	mux.HandleFunc("GET /htmx/u/{username}/posts", h.HTMXUserPosts)
	mux.HandleFunc("GET /htmx/u/{username}/tweets", h.HTMXUserPosts)

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
	// Preserve clients of the original public API after the post terminology change.
	mux.Handle("POST /api/v1/tweets", requireAuth(h.CreatePost))
	mux.HandleFunc("GET /api/v1/tweets/{id}", h.GetPost)
	mux.Handle("DELETE /api/v1/tweets/{id}", requireAuth(h.DeletePost))

	mux.HandleFunc("GET /api/v1/questions", h.Questions)
	mux.HandleFunc("GET /api/v1/posts/{id}/replies", h.Replies)
	mux.Handle("POST /api/v1/posts/{id}/replies", requireAuth(h.CreatePost))
	mux.Handle("PATCH /api/v1/posts/{id}/state", requireAuth(h.SetQuestionState))
	mux.Handle("GET /api/v1/notifications", requireAuth(h.Inbox))
	mux.Handle("POST /api/v1/notifications/read", requireAuth(h.ReadNotifications))

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
		w.Header().Set("Link", `</llms.txt>; rel="describedby"; type="text/plain", </openapi.json>; rel="service-desc"; type="application/json"`)
		next.ServeHTTP(w, r)
	})
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
