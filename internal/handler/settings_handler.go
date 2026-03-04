package handler

import (
	"net/http"
	"strings"

	"github.com/drewangeloff/old_school_bird/internal/ctxutil"
	"github.com/drewangeloff/old_school_bird/internal/tmpl"
)

func (h *Handler) ShowSettings(w http.ResponseWriter, r *http.Request) {
	user := ctxutil.GetUser(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	h.Renderer.Render(w, r, "settings.html", tmpl.PageData{
		Title: "Settings",
		User:  user,
		Data:  user,
	})
}

func (h *Handler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	user := ctxutil.GetUser(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.Renderer.Render(w, r, "settings.html", tmpl.PageData{
			Title: "Settings", User: user, Data: user,
			Flash: "Invalid form data", FlashType: "error",
		})
		return
	}

	displayName := strings.TrimSpace(r.FormValue("display_name"))
	bio := strings.TrimSpace(r.FormValue("bio"))

	if displayName == "" {
		displayName = user.Username
	}
	if len(displayName) > 50 {
		h.Renderer.Render(w, r, "settings.html", tmpl.PageData{
			Title: "Settings", User: user, Data: user,
			Flash: "Display name must be under 50 characters", FlashType: "error",
		})
		return
	}
	if len(bio) > 160 {
		h.Renderer.Render(w, r, "settings.html", tmpl.PageData{
			Title: "Settings", User: user, Data: user,
			Flash: "Bio must be under 160 characters", FlashType: "error",
		})
		return
	}

	user.DisplayName = displayName
	user.Bio = bio

	if err := h.Repo.UpdateUser(r.Context(), user); err != nil {
		h.Renderer.Render(w, r, "settings.html", tmpl.PageData{
			Title: "Settings", User: user, Data: user,
			Flash: "Failed to update profile", FlashType: "error",
		})
		return
	}

	h.Renderer.Render(w, r, "settings.html", tmpl.PageData{
		Title: "Settings", User: user, Data: user,
		Flash: "Profile updated", FlashType: "success",
	})
}
