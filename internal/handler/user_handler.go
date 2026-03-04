package handler

import (
	"encoding/json"
	"net/http"

	"github.com/drewangeloff/old_school_bird/internal/ctxutil"
	"github.com/drewangeloff/old_school_bird/internal/tmpl"
)

func (h *Handler) Profile(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	profileUser, err := h.Repo.GetUserByUsername(r.Context(), username)
	if err != nil || profileUser == nil {
		http.NotFound(w, r)
		return
	}

	stats, err := h.Repo.GetUserStats(r.Context(), profileUser.ID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	tweets, cursor, err := h.Repo.GetTweetsByUserID(r.Context(), profileUser.ID, r.URL.Query().Get("cursor"), 20)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	currentUser := ctxutil.GetUser(r.Context())
	markDeletable(tweets, currentUser)
	var isFollowing bool
	if currentUser != nil && currentUser.ID != profileUser.ID {
		isFollowing, err = h.Repo.IsFollowing(r.Context(), currentUser.ID, profileUser.ID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}

	data := map[string]any{
		"ProfileUser": profileUser,
		"Stats":       stats,
		"Tweets":      tweets,
		"NextCursor":  cursor,
		"IsFollowing": isFollowing,
		"IsOwnProfile": currentUser != nil && currentUser.ID == profileUser.ID,
	}

	if tmpl.WantsJSON(r) {
		// Use PublicProfile to avoid leaking email; include email only for own profile
		profileData := profileUser.PublicProfile()
		if currentUser != nil && currentUser.ID == profileUser.ID {
			profileData = profileUser.OwnProfile()
		}
		data["ProfileUser"] = profileData
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data)
		return
	}

	h.Renderer.Render(w, r, "profile.html", tmpl.PageData{
		Title:      "@" + profileUser.Username,
		User:       currentUser,
		Data:       data,
		NextCursor: cursor,
	})
}

func (h *Handler) Follow(w http.ResponseWriter, r *http.Request) {
	user := ctxutil.GetUser(r.Context())
	if user == nil {
		if tmpl.WantsJSON(r) {
			h.jsonError(w, "unauthorized", http.StatusUnauthorized)
		} else {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
		}
		return
	}

	username := r.PathValue("username")
	target, err := h.Repo.GetUserByUsername(r.Context(), username)
	if err != nil || target == nil {
		http.NotFound(w, r)
		return
	}

	if user.ID == target.ID {
		if tmpl.WantsJSON(r) {
			h.jsonError(w, "cannot follow yourself", http.StatusBadRequest)
		} else {
			http.Error(w, "cannot follow yourself", http.StatusBadRequest)
		}
		return
	}

	if err := h.Repo.Follow(r.Context(), user.ID, target.ID); err != nil {
		h.jsonError(w, "failed to follow", http.StatusInternalServerError)
		return
	}

	if tmpl.WantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"following": true})
		return
	}

	// HTMX: return updated follow button
	h.Renderer.RenderPartial(w, r, "follow_button.html", map[string]any{
		"Username":    username,
		"IsFollowing": true,
	})
}

func (h *Handler) Unfollow(w http.ResponseWriter, r *http.Request) {
	user := ctxutil.GetUser(r.Context())
	if user == nil {
		if tmpl.WantsJSON(r) {
			h.jsonError(w, "unauthorized", http.StatusUnauthorized)
		} else {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
		}
		return
	}

	username := r.PathValue("username")
	target, err := h.Repo.GetUserByUsername(r.Context(), username)
	if err != nil || target == nil {
		http.NotFound(w, r)
		return
	}

	if err := h.Repo.Unfollow(r.Context(), user.ID, target.ID); err != nil {
		h.jsonError(w, "failed to unfollow", http.StatusInternalServerError)
		return
	}

	if tmpl.WantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"following": false})
		return
	}

	h.Renderer.RenderPartial(w, r, "follow_button.html", map[string]any{
		"Username":    username,
		"IsFollowing": false,
	})
}

func (h *Handler) HTMXUserTweets(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	profileUser, err := h.Repo.GetUserByUsername(r.Context(), username)
	if err != nil || profileUser == nil {
		http.NotFound(w, r)
		return
	}
	currentUser := ctxutil.GetUser(r.Context())
	cursor := r.URL.Query().Get("cursor")
	tweets, nextCursor, err := h.Repo.GetTweetsByUserID(r.Context(), profileUser.ID, cursor, 20)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	markDeletable(tweets, currentUser)
	h.Renderer.RenderPartial(w, r, "tweet_list.html", map[string]any{
		"Tweets":      tweets,
		"NextCursor":  nextCursor,
		"TimelineURL": "/htmx/u/" + username + "/tweets",
	})
}
