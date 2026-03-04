package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/drewangeloff/vostros/internal/auth"
	"github.com/drewangeloff/vostros/internal/ctxutil"
	"github.com/drewangeloff/vostros/internal/model"
	"github.com/drewangeloff/vostros/internal/tmpl"
)

func (h *Handler) CreatePost(w http.ResponseWriter, r *http.Request) {
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
			h.jsonError(w, "post must be 1-256 characters", http.StatusBadRequest)
		} else {
			http.Error(w, "post must be 1-256 characters", http.StatusBadRequest)
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

	post := &model.Post{
		ID:        auth.NewULID(),
		UserID:    user.ID,
		Content:   content,
		Status:    "visible",
		CreatedAt: time.Now(),
		User:      user,
	}

	if err := h.Repo.CreatePostWithOutbox(r.Context(), post); err != nil {
		if tmpl.WantsJSON(r) {
			h.jsonError(w, "failed to create post", http.StatusInternalServerError)
		} else {
			http.Error(w, "failed to create post", http.StatusInternalServerError)
		}
		return
	}

	if tmpl.WantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(post)
		return
	}

	// HTMX: return the post partial to prepend
	post.CanDelete = true
	h.Renderer.RenderPartial(w, r, "post.html", post)
}

func (h *Handler) DeletePost(w http.ResponseWriter, r *http.Request) {
	user := ctxutil.GetUser(r.Context())
	if user == nil {
		if tmpl.WantsJSON(r) {
			h.jsonError(w, "unauthorized", http.StatusUnauthorized)
		} else {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
		}
		return
	}

	postID := r.PathValue("id")
	post, err := h.Repo.GetPostByID(r.Context(), postID)
	if err != nil || post == nil {
		http.NotFound(w, r)
		return
	}

	if post.UserID != user.ID && user.Role != "admin" {
		h.jsonError(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := h.Repo.DeletePost(r.Context(), postID); err != nil {
		h.jsonError(w, "failed to delete", http.StatusInternalServerError)
		return
	}

	if tmpl.WantsJSON(r) {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("HX-Trigger", `{"showToast":{"message":"Post deleted","type":"success"}}`)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) GetPost(w http.ResponseWriter, r *http.Request) {
	postID := r.PathValue("id")
	post, err := h.Repo.GetPostByID(r.Context(), postID)
	if err != nil || post == nil || post.Status != "visible" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(post)
}
