package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/drewangeloff/old_school_bird/internal/auth"
	"github.com/drewangeloff/old_school_bird/internal/ctxutil"
	"github.com/drewangeloff/old_school_bird/internal/model"
	"github.com/drewangeloff/old_school_bird/internal/tmpl"
)

func (h *Handler) PostTweet(w http.ResponseWriter, r *http.Request) {
	user := ctxutil.GetUser(r.Context())
	if user == nil {
		if tmpl.WantsJSON(r) {
			h.jsonError(w, "unauthorized", http.StatusUnauthorized)
		} else {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
		}
		return
	}

	var content string
	if tmpl.WantsJSON(r) {
		var input struct {
			Content string `json:"content"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			h.jsonError(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		content = input.Content
	} else {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form data", http.StatusBadRequest)
			return
		}
		content = r.FormValue("content")
	}

	content = strings.TrimSpace(content)
	if content == "" || len(content) > 256 {
		if tmpl.WantsJSON(r) {
			h.jsonError(w, "tweet must be 1-256 characters", http.StatusBadRequest)
		} else {
			http.Error(w, "tweet must be 1-256 characters", http.StatusBadRequest)
		}
		return
	}

	// Sync moderation
	if h.Moderator != nil {
		if ok, reason := h.Moderator.Check(content); !ok {
			if tmpl.WantsJSON(r) {
				h.jsonError(w, reason, http.StatusForbidden)
			} else {
				http.Error(w, reason, http.StatusForbidden)
			}
			return
		}
	}

	tweet := &model.Tweet{
		ID:        auth.NewULID(),
		UserID:    user.ID,
		Content:   content,
		Status:    "visible",
		CreatedAt: time.Now(),
		User:      user,
	}

	if err := h.Repo.CreateTweetWithOutbox(r.Context(), tweet); err != nil {
		if tmpl.WantsJSON(r) {
			h.jsonError(w, "failed to create tweet", http.StatusInternalServerError)
		} else {
			http.Error(w, "failed to create tweet", http.StatusInternalServerError)
		}
		return
	}

	if tmpl.WantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(tweet)
		return
	}

	// HTMX: return the tweet partial to prepend
	tweet.CanDelete = true
	h.Renderer.RenderPartial(w, r, "tweet.html", tweet)
}

func (h *Handler) DeleteTweet(w http.ResponseWriter, r *http.Request) {
	user := ctxutil.GetUser(r.Context())
	if user == nil {
		if tmpl.WantsJSON(r) {
			h.jsonError(w, "unauthorized", http.StatusUnauthorized)
		} else {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
		}
		return
	}

	tweetID := r.PathValue("id")
	tweet, err := h.Repo.GetTweetByID(r.Context(), tweetID)
	if err != nil || tweet == nil {
		http.NotFound(w, r)
		return
	}

	if tweet.UserID != user.ID && user.Role != "admin" {
		h.jsonError(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := h.Repo.DeleteTweet(r.Context(), tweetID); err != nil {
		h.jsonError(w, "failed to delete", http.StatusInternalServerError)
		return
	}

	if tmpl.WantsJSON(r) {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// HTMX: return empty to remove the tweet from DOM
	w.Header().Set("HX-Trigger", `{"showToast":{"message":"Tweet deleted","type":"success"}}`)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) GetTweet(w http.ResponseWriter, r *http.Request) {
	tweetID := r.PathValue("id")
	tweet, err := h.Repo.GetTweetByID(r.Context(), tweetID)
	if err != nil || tweet == nil || tweet.Status != "visible" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tweet)
}
