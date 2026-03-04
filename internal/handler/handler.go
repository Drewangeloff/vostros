package handler

import (
	"encoding/json"
	"net/http"

	"github.com/drewangeloff/vostros/internal/auth"
	"github.com/drewangeloff/vostros/internal/model"
	"github.com/drewangeloff/vostros/internal/moderation"
	"github.com/drewangeloff/vostros/internal/repository"
	"github.com/drewangeloff/vostros/internal/tmpl"
)

type Handler struct {
	Repo      repository.Repository
	Renderer  *tmpl.Renderer
	Auth      *auth.Service
	Moderator moderation.Moderator
}

func New(repo repository.Repository, renderer *tmpl.Renderer, authService *auth.Service, mod moderation.Moderator) *Handler {
	return &Handler{Repo: repo, Renderer: renderer, Auth: authService, Moderator: mod}
}

// maxBodySize is the maximum allowed request body size (1 MB).
const maxBodySize = 1 << 20

func (h *Handler) jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func markDeletable(tweets []*model.Tweet, user *model.User) {
	if user == nil {
		return
	}
	for _, t := range tweets {
		if t.UserID == user.ID || user.Role == "admin" {
			t.CanDelete = true
		}
	}
}

func (h *Handler) NotFound(w http.ResponseWriter, r *http.Request) {
	if tmpl.WantsJSON(r) {
		h.jsonError(w, "not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNotFound)
	h.Renderer.Render(w, r, "error.html", tmpl.PageData{
		Title: "Not Found",
		Data: map[string]any{
			"Code":    "404",
			"Message": "The page you're looking for doesn't exist.",
		},
	})
}

func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	if err := h.Repo.Ping(r.Context()); err != nil {
		http.Error(w, "unhealthy", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
