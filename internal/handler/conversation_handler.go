package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/drewangeloff/vostros/internal/ctxutil"
	"github.com/drewangeloff/vostros/internal/model"
	"github.com/drewangeloff/vostros/internal/tmpl"
)

func (h *Handler) requestError(w http.ResponseWriter, r *http.Request, message string, status int) {
	if tmpl.WantsJSON(r) {
		h.jsonError(w, message, status)
	} else {
		http.Error(w, message, status)
	}
}

func (h *Handler) redirectAfterWrite(w http.ResponseWriter, r *http.Request, path string) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", path)
		w.WriteHeader(http.StatusOK)
	} else {
		http.Redirect(w, r, path, http.StatusSeeOther)
	}
}

func validQuestionState(state string) bool {
	return state == "open" || state == "answered" || state == "tested"
}

func (h *Handler) Questions(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state == "" {
		state = "open"
	}
	if state != "all" && !validQuestionState(state) {
		h.requestError(w, r, "state must be open, answered, tested, or all", 400)
		return
	}
	posts, cursor, err := h.Repo.GetQuestions(r.Context(), state, r.URL.Query().Get("cursor"), 20)
	if err != nil {
		h.requestError(w, r, "could not load questions", 500)
		return
	}
	if posts == nil {
		posts = []*model.Post{}
	}
	if len(posts) < 20 {
		cursor = ""
	}
	user := ctxutil.GetUser(r.Context())
	markDeletable(posts, user)
	if tmpl.WantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"posts": posts, "next_cursor": cursor, "state": state})
		return
	}
	h.Renderer.Render(w, r, "questions.html", tmpl.PageData{Title: "Ask agents", CanonicalPath: "/questions", User: user,
		Data: map[string]any{"Posts": posts, "State": state, "NextCursor": cursor}})
}

func (h *Handler) Replies(w http.ResponseWriter, r *http.Request) {
	post, err := h.Repo.GetPostByID(r.Context(), r.PathValue("id"))
	if err != nil {
		h.jsonError(w, "could not load conversation", 500)
		return
	}
	if post == nil || post.Status != "visible" {
		h.NotFound(w, r)
		return
	}
	threadID := post.ID
	if post.ThreadID != "" {
		threadID = post.ThreadID
	}
	replies, cursor, err := h.Repo.GetReplies(r.Context(), threadID, r.URL.Query().Get("cursor"), 20)
	if err != nil {
		h.jsonError(w, "could not load replies", 500)
		return
	}
	if replies == nil {
		replies = []*model.Post{}
	}
	if len(replies) < 20 {
		cursor = ""
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"posts": replies, "thread_id": threadID, "next_cursor": cursor})
}

func (h *Handler) SetQuestionState(w http.ResponseWriter, r *http.Request) {
	user := ctxutil.GetUser(r.Context())
	if user == nil {
		h.requestError(w, r, "unauthorized", 401)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	var state string
	if tmpl.WantsJSON(r) {
		var input struct {
			State string `json:"state"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			h.jsonError(w, "invalid JSON", 400)
			return
		}
		state = input.State
	} else {
		if err := r.ParseForm(); err != nil {
			h.requestError(w, r, "invalid form", 400)
			return
		}
		state = r.FormValue("state")
	}
	if !validQuestionState(state) {
		h.requestError(w, r, "state must be open, answered, or tested", 400)
		return
	}
	post, err := h.Repo.GetPostByID(r.Context(), r.PathValue("id"))
	if err != nil {
		h.requestError(w, r, "could not load question", 500)
		return
	}
	if post == nil || post.Status != "visible" {
		h.NotFound(w, r)
		return
	}
	if post.UserID != user.ID {
		h.requestError(w, r, "only the question author can update its state", 403)
		return
	}
	if post.Kind != "question" {
		h.requestError(w, r, "this post is not a question", 400)
		return
	}
	updated, err := h.Repo.SetQuestionState(r.Context(), post.ID, user.ID, state)
	if err != nil {
		h.requestError(w, r, "could not update question", 500)
		return
	}
	if !updated {
		h.NotFound(w, r)
		return
	}
	if tmpl.WantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": post.ID, "question_state": state})
	} else {
		h.redirectAfterWrite(w, r, "/p/"+post.ID)
	}
}

func (h *Handler) Inbox(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	user := ctxutil.GetUser(r.Context())
	if user == nil {
		h.requestError(w, r, "unauthorized", 401)
		return
	}
	unread := r.URL.Query().Get("unread") != "false"
	if value := r.URL.Query().Get("unread"); value != "" && value != "true" && value != "false" {
		h.requestError(w, r, "unread must be true or false", 400)
		return
	}
	var cursor int64
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed <= 0 {
			h.requestError(w, r, "invalid cursor", 400)
			return
		}
		cursor = parsed
	}
	items, next, err := h.Repo.GetNotifications(r.Context(), user.ID, unread, cursor, 20)
	if err != nil {
		h.requestError(w, r, "could not load inbox", 500)
		return
	}
	count, err := h.Repo.CountUnreadNotifications(r.Context(), user.ID)
	if err != nil {
		h.requestError(w, r, "could not load unread count", 500)
		return
	}
	if items == nil {
		items = []*model.Notification{}
	}
	if tmpl.WantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"notifications": items, "unread_count": count, "next_cursor": next})
		return
	}
	h.Renderer.Render(w, r, "inbox.html", tmpl.PageData{Title: "Needs your attention", User: user,
		Data: map[string]any{"Items": items, "Unread": unread, "UnreadCount": count, "NextCursor": next}})
}

func (h *Handler) InboxCount(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	user := ctxutil.GetUser(r.Context())
	if user == nil {
		h.requestError(w, r, "unauthorized", 401)
		return
	}
	count, err := h.Repo.CountUnreadNotifications(r.Context(), user.ID)
	if err != nil {
		h.requestError(w, r, "unavailable", 500)
		return
	}
	h.Renderer.RenderPartial(w, r, "inbox_count.html", count)
}

func (h *Handler) ReadNotifications(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	user := ctxutil.GetUser(r.Context())
	if user == nil {
		h.requestError(w, r, "unauthorized", 401)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	var rawIDs []string
	if tmpl.WantsJSON(r) {
		var input struct {
			IDs []string `json:"ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			h.jsonError(w, "invalid JSON; ids must be an array of strings", 400)
			return
		}
		rawIDs = input.IDs
	} else {
		if err := r.ParseForm(); err != nil {
			h.requestError(w, r, "invalid form", 400)
			return
		}
		rawIDs = r.Form["ids"]
	}
	if len(rawIDs) == 0 || len(rawIDs) > 100 {
		h.requestError(w, r, "provide 1-100 notification IDs", 400)
		return
	}
	ids := make([]int64, 0, len(rawIDs))
	for _, raw := range rawIDs {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			h.requestError(w, r, "invalid notification ID", 400)
			return
		}
		ids = append(ids, id)
	}
	count, err := h.Repo.ReadNotifications(r.Context(), user.ID, ids)
	if err != nil {
		h.requestError(w, r, "could not mark notifications read", 500)
		return
	}
	if tmpl.WantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int64{"marked_read": count})
	} else {
		h.redirectAfterWrite(w, r, "/inbox")
	}
}
