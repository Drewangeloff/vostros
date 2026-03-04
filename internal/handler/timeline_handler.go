package handler

import (
	"net/http"

	"github.com/drewangeloff/old_school_bird/internal/ctxutil"
	"github.com/drewangeloff/old_school_bird/internal/tmpl"
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
	tweets, cursor, err := h.Repo.GetGlobalTimeline(r.Context(), "", 20)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.Renderer.Render(w, r, "home.html", tmpl.PageData{
		Title:      "Welcome",
		Data:       tweets,
		NextCursor: cursor,
	})
}

func (h *Handler) Timeline(w http.ResponseWriter, r *http.Request) {
	user := ctxutil.GetUser(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	cursor := r.URL.Query().Get("cursor")
	tweets, nextCursor, err := h.Repo.GetHomeTimeline(r.Context(), user.ID, cursor, 20)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	markDeletable(tweets, user)
	h.Renderer.Render(w, r, "timeline.html", tmpl.PageData{
		Title:      "Home",
		User:       user,
		Data:       tweets,
		NextCursor: nextCursor,
	})
}

func (h *Handler) Global(w http.ResponseWriter, r *http.Request) {
	user := ctxutil.GetUser(r.Context())
	cursor := r.URL.Query().Get("cursor")
	tweets, nextCursor, err := h.Repo.GetGlobalTimeline(r.Context(), cursor, 20)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	markDeletable(tweets, user)
	h.Renderer.Render(w, r, "global.html", tmpl.PageData{
		Title:      "Global",
		User:       user,
		Data:       tweets,
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
	tweets, nextCursor, err := h.Repo.GetHomeTimeline(r.Context(), user.ID, cursor, 20)
	if err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	markDeletable(tweets, user)
	h.Renderer.RenderPartial(w, r, "tweet_list.html", map[string]any{
		"Tweets":     tweets,
		"NextCursor": nextCursor,
		"TimelineURL": "/htmx/timeline",
	})
}

func (h *Handler) HTMXGlobal(w http.ResponseWriter, r *http.Request) {
	user := ctxutil.GetUser(r.Context())
	cursor := r.URL.Query().Get("cursor")
	tweets, nextCursor, err := h.Repo.GetGlobalTimeline(r.Context(), cursor, 20)
	if err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	markDeletable(tweets, user)
	h.Renderer.RenderPartial(w, r, "tweet_list.html", map[string]any{
		"Tweets":     tweets,
		"NextCursor": nextCursor,
		"TimelineURL": "/htmx/global",
	})
}
