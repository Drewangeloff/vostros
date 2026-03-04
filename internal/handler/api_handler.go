package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/drewangeloff/vostros/internal/ctxutil"
	"github.com/drewangeloff/vostros/internal/model"
	"github.com/drewangeloff/vostros/internal/tmpl"
	"github.com/oklog/ulid/v2"
)

func (h *Handler) ShowAPI(w http.ResponseWriter, r *http.Request) {
	user := ctxutil.GetUser(r.Context())

	tokens, err := h.Repo.ListAPITokensByUser(r.Context(), user.ID)
	if err != nil {
		h.Renderer.Render(w, r, "developers.html", tmpl.PageData{
			Title: "API",
			User:  user,
			Data:  map[string]any{"Tokens": []*model.APIToken{}, "Error": "Failed to load tokens"},
		})
		return
	}
	if tokens == nil {
		tokens = []*model.APIToken{}
	}

	h.Renderer.Render(w, r, "developers.html", tmpl.PageData{
		Title: "API",
		User:  user,
		Data:  map[string]any{"Tokens": tokens},
	})
}

func (h *Handler) CreateAPIToken(w http.ResponseWriter, r *http.Request) {
	user := ctxutil.GetUser(r.Context())

	var name string
	if tmpl.WantsJSON(r) {
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			h.jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		name = strings.TrimSpace(body.Name)
	} else {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		name = strings.TrimSpace(r.FormValue("name"))
	}

	if name == "" || len(name) > 64 {
		if tmpl.WantsJSON(r) {
			h.jsonError(w, "name is required (max 64 chars)", http.StatusBadRequest)
			return
		}
		w.Header().Set("HX-Trigger", `{"showToast":{"message":"Token name is required (max 64 chars)","type":"error"}}`)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Generate token: vst_ + base64url(32 random bytes)
	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		if tmpl.WantsJSON(r) {
			h.jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	plaintext := "vst_" + base64.RawURLEncoding.EncodeToString(rawBytes)

	// Hash for storage
	hash := sha256.Sum256([]byte(plaintext))
	hashHex := hex.EncodeToString(hash[:])

	// Prefix for display: first 8 chars
	prefix := plaintext[:8]

	now := time.Now().UTC()
	token := &model.APIToken{
		ID:        ulid.Make().String(),
		UserID:    user.ID,
		Name:      name,
		TokenHash: hashHex,
		Prefix:    prefix,
		CreatedAt: now,
	}

	if err := h.Repo.CreateAPIToken(r.Context(), token); err != nil {
		if tmpl.WantsJSON(r) {
			h.jsonError(w, "failed to create token", http.StatusInternalServerError)
			return
		}
		w.Header().Set("HX-Trigger", `{"showToast":{"message":"Failed to create token","type":"error"}}`)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if tmpl.WantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"token": plaintext,
			"id":    token.ID,
			"name":  token.Name,
			"prefix": token.Prefix,
		})
		return
	}

	// HTMX: return the one-time token display partial
	h.Renderer.RenderPartial(w, r, "new_token.html", map[string]any{
		"Token":     plaintext,
		"TokenName": name,
		"TokenID":   token.ID,
		"Prefix":    token.Prefix,
		"CreatedAt": token.CreatedAt,
	})
}

func (h *Handler) DeleteAPIToken(w http.ResponseWriter, r *http.Request) {
	user := ctxutil.GetUser(r.Context())
	id := r.PathValue("id")

	if err := h.Repo.DeleteAPIToken(r.Context(), id, user.ID); err != nil {
		if tmpl.WantsJSON(r) {
			h.jsonError(w, "token not found", http.StatusNotFound)
			return
		}
		w.Header().Set("HX-Trigger", `{"showToast":{"message":"Token not found","type":"error"}}`)
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if tmpl.WantsJSON(r) {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("HX-Trigger", `{"showToast":{"message":"Token deleted","type":"success"}}`)
	w.WriteHeader(http.StatusOK)
}
