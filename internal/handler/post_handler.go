package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/drewangeloff/vostros/internal/auth"
	"github.com/drewangeloff/vostros/internal/ctxutil"
	"github.com/drewangeloff/vostros/internal/model"
	"github.com/drewangeloff/vostros/internal/repository"
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

	var content, kind string
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	if tmpl.WantsJSON(r) {
		var input struct {
			Content string `json:"content"`
			Kind    string `json:"kind"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			h.jsonError(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		content = input.Content
		kind = input.Kind
	} else {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form data", http.StatusBadRequest)
			return
		}
		content = r.FormValue("content")
		kind = r.FormValue("kind")
	}

	if kind == "" {
		kind = "post"
	}
	parentID := r.PathValue("id")
	if kind != "post" && kind != "question" || parentID != "" && kind != "post" {
		h.requestError(w, r, "kind must be post or question; replies must be posts", http.StatusBadRequest)
		return
	}
	content = strings.TrimSpace(content)
	if content == "" || utf8.RuneCountInString(content) > 256 {
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
		Kind:      kind,
		ParentID:  parentID,
	}

	if kind == "question" {
		post.QuestionState = "open"
	}

	if err := h.Repo.CreatePostWithOutbox(r.Context(), post); err != nil {
		if errors.Is(err, repository.ErrPostUnavailable) {
			h.NotFound(w, r)
			return
		}
		if tmpl.WantsJSON(r) {
			h.jsonError(w, "failed to create post", http.StatusInternalServerError)
		} else {
			http.Error(w, "failed to create post", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Location", "/p/"+post.ID)
	if tmpl.WantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(post)
		return
	}

	if parentID != "" || kind == "question" {
		h.redirectAfterWrite(w, r, "/p/"+post.ID)
		return
	}
	if r.Header.Get("HX-Request") != "true" {
		http.Redirect(w, r, "/p/"+post.ID, http.StatusSeeOther)
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

	if current, err := url.Parse(r.Header.Get("HX-Current-URL")); err == nil && strings.HasPrefix(current.Path, "/p/") {
		target := "/global"
		if post.ThreadID != "" {
			target = "/p/" + post.ThreadID
		} else if post.Kind == "question" {
			target = "/questions"
		}
		h.redirectAfterWrite(w, r, target)
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

func (h *Handler) ShowPost(w http.ResponseWriter, r *http.Request) {
	post, err := h.Repo.GetPostByID(r.Context(), r.PathValue("id"))
	if err != nil {
		h.requestError(w, r, "could not load this conversation", 500)
		return
	}
	if post == nil || post.Status != "visible" {
		h.NotFound(w, r)
		return
	}
	thread := post
	if post.ThreadID != "" {
		thread, err = h.Repo.GetPostByID(r.Context(), post.ThreadID)
		if err != nil {
			h.requestError(w, r, "could not load this conversation", 500)
			return
		}
		if thread == nil || thread.Status != "visible" {
			h.NotFound(w, r)
			return
		}
	}
	replies, cursor, err := h.Repo.GetReplies(r.Context(), thread.ID, r.URL.Query().Get("cursor"), 20)
	if err != nil {
		h.requestError(w, r, "could not load replies", 500)
		return
	}
	if len(replies) < 20 {
		cursor = ""
	}
	user := ctxutil.GetUser(r.Context())
	markDeletable(append([]*model.Post{thread, post}, replies...), user)
	outside := post.ID != thread.ID
	for _, reply := range replies {
		if reply.ID == post.ID {
			outside = false
		}
	}
	h.Renderer.Render(w, r, "post_page.html", tmpl.PageData{
		Title: "Conversation with @" + thread.User.Username, Description: thread.Content,
		CanonicalPath: "/p/" + post.ID, User: user,
		Data: map[string]any{"Thread": thread, "ReplyTo": post, "Replies": replies, "NextCursor": cursor,
			"ReplyOutsidePage": outside, "CanSetState": user != nil && user.ID == thread.UserID},
	})
}
