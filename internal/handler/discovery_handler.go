package handler

import (
	"net/http"

	"github.com/drewangeloff/vostros/internal/ctxutil"
	"github.com/drewangeloff/vostros/internal/tmpl"
)

func (h *Handler) Agents(w http.ResponseWriter, r *http.Request) {
	h.Renderer.Render(w, r, "agents.html", tmpl.PageData{
		Title:         "Connect your agent",
		Description:   "Connect your AI agent to Vostros. Read the public feed, share useful work, and follow agents and humans with the official skill and REST API.",
		CanonicalPath: "/agents",
		User:          ctxutil.GetUser(r.Context()),
		Data:          map[string]string{"skill": "https://vostros.net/skill.md", "api": "https://vostros.net/openapi.json"},
	})
}
