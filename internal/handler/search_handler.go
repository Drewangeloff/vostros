package handler

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/drewangeloff/old_school_bird/internal/ctxutil"
	"github.com/drewangeloff/old_school_bird/internal/tmpl"
)

func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	cursor := r.URL.Query().Get("cursor")
	user := ctxutil.GetUser(r.Context())

	results, nextCursor, err := h.Repo.SearchTweets(r.Context(), query, cursor, 20)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	markDeletable(results, user)

	var users []*any
	if query != "" && cursor == "" {
		foundUsers, err := h.Repo.SearchUsers(r.Context(), query, 5)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		for _, u := range foundUsers {
			v := any(u)
			users = append(users, &v)
		}
	}

	if tmpl.WantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"tweets": results,
			"users":  users,
		})
		return
	}

	data := map[string]any{
		"Query":   query,
		"Results": results,
		"Users":   users,
	}

	h.Renderer.Render(w, r, "search.html", tmpl.PageData{
		Title:      "Search",
		User:       user,
		Data:       data,
		NextCursor: nextCursor,
	})
}

func (h *Handler) HTMXSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	cursor := r.URL.Query().Get("cursor")
	user := ctxutil.GetUser(r.Context())

	if query == "" {
		w.WriteHeader(http.StatusOK)
		return
	}

	tweets, nextCursor, err := h.Repo.SearchTweets(r.Context(), query, cursor, 20)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	markDeletable(tweets, user)

	// On first search (no cursor), also search users
	var foundUsers interface{}
	if cursor == "" {
		u, err := h.Repo.SearchUsers(r.Context(), query, 5)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		foundUsers = u
	}

	h.Renderer.RenderPartial(w, r, "search_results.html", map[string]any{
		"Query":       query,
		"Tweets":      tweets,
		"Users":       foundUsers,
		"NextCursor":  nextCursor,
		"TimelineURL": "/htmx/search?q=" + url.QueryEscape(query),
	})
}
