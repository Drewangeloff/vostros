package handler

import (
	"net/http"

	"github.com/drewangeloff/vostros/internal/ctxutil"
	"github.com/drewangeloff/vostros/internal/tmpl"
)

func (h *Handler) Home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	user := ctxutil.GetUser(r.Context())
	if user != nil {
		http.Redirect(w, r, "/timeline", http.StatusSeeOther)
		return
	}

	// Logged out — show global timeline as landing
	posts, cursor, err := h.Repo.GetGlobalTimeline(r.Context(), "", 20)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.Renderer.Render(w, r, "home.html", tmpl.PageData{
		Title:      "Welcome",
		Data:       posts,
		NextCursor: cursor,
	})
}

func (h *Handler) Timeline(w http.ResponseWriter, r *http.Request) {
	user := ctxutil.GetUser(r.Context())
	if user == nil {
		if tmpl.WantsJSON(r) {
			h.jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	cursor := r.URL.Query().Get("cursor")
	posts, nextCursor, err := h.Repo.GetHomeTimeline(r.Context(), user.ID, cursor, 20)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	markDeletable(posts, user)
	h.Renderer.Render(w, r, "timeline.html", tmpl.PageData{
		Title:      "Home",
		User:       user,
		Data:       posts,
		NextCursor: nextCursor,
	})
}

func (h *Handler) Global(w http.ResponseWriter, r *http.Request) {
	user := ctxutil.GetUser(r.Context())
	cursor := r.URL.Query().Get("cursor")
	posts, nextCursor, err := h.Repo.GetGlobalTimeline(r.Context(), cursor, 20)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	markDeletable(posts, user)
	h.Renderer.Render(w, r, "global.html", tmpl.PageData{
		Title:      "Global",
		User:       user,
		Data:       posts,
		NextCursor: nextCursor,
	})
}

func (h *Handler) HTMXTimeline(w http.ResponseWriter, r *http.Request) {
	user := ctxutil.GetUser(r.Context())
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	cursor := r.URL.Query().Get("cursor")
	posts, nextCursor, err := h.Repo.GetHomeTimeline(r.Context(), user.ID, cursor, 20)
	if err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	markDeletable(posts, user)
	h.Renderer.RenderPartial(w, r, "post_list.html", map[string]any{
		"Posts":       posts,
		"NextCursor":  nextCursor,
		"TimelineURL": "/htmx/timeline",
	})
}

func (h *Handler) HTMXGlobal(w http.ResponseWriter, r *http.Request) {
	user := ctxutil.GetUser(r.Context())
	cursor := r.URL.Query().Get("cursor")
	posts, nextCursor, err := h.Repo.GetGlobalTimeline(r.Context(), cursor, 20)
	if err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	markDeletable(posts, user)
	h.Renderer.RenderPartial(w, r, "post_list.html", map[string]any{
		"Posts":       posts,
		"NextCursor":  nextCursor,
		"TimelineURL": "/htmx/global",
	})
}
