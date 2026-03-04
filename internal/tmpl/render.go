package tmpl

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"
)

type Renderer struct {
	templates *template.Template
	devMode   bool
}

type PageData struct {
	Title      string
	User       any
	Data       any
	Flash      string
	FlashType  string
	NextCursor string
}

func New(templateFS embed.FS, devMode bool) *Renderer {
	funcMap := template.FuncMap{
		"timeago": timeAgo,
		"slice": func(s string, start, end int) string {
			if end > len(s) {
				end = len(s)
			}
			if start > len(s) {
				return ""
			}
			return s[start:end]
		},
		"map": func(pairs ...any) map[string]any {
			m := make(map[string]any)
			for i := 0; i < len(pairs)-1; i += 2 {
				m[fmt.Sprintf("%v", pairs[i])] = pairs[i+1]
			}
			return m
		},
	}

	r := &Renderer{devMode: devMode}

	tmpl := template.New("").Funcs(funcMap)
	err := fs.WalkDir(templateFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		content, err := templateFS.ReadFile(path)
		if err != nil {
			return err
		}
		name := path
		if idx := strings.LastIndex(path, "/"); idx >= 0 {
			name = path[idx+1:]
		}
		_, err = tmpl.New(name).Parse(string(content))
		return err
	})
	if err != nil {
		log.Fatalf("parsing templates: %v", err)
	}
	r.templates = tmpl
	return r
}

func (r *Renderer) Render(w http.ResponseWriter, req *http.Request, page string, data PageData) {
	if WantsJSON(req) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data.Data)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// HTMX partial — render just the page fragment
	if req.Header.Get("HX-Request") == "true" {
		if err := r.templates.ExecuteTemplate(w, page, data); err != nil {
			log.Printf("template error (partial %s): %v", page, err)
			http.Error(w, "template error", http.StatusInternalServerError)
		}
		return
	}

	// Full page — render page to buffer, inject into layout
	var buf bytes.Buffer
	if err := r.templates.ExecuteTemplate(&buf, page, data); err != nil {
		log.Printf("template error (page %s): %v", page, err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	layoutData := map[string]any{
		"Title":     data.Title,
		"User":      data.User,
		"Flash":     data.Flash,
		"FlashType": data.FlashType,
		"Content":   template.HTML(buf.String()),
	}
	if err := r.templates.ExecuteTemplate(w, "layout.html", layoutData); err != nil {
		log.Printf("template error (layout for %s): %v", page, err)
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (r *Renderer) RenderPartial(w http.ResponseWriter, req *http.Request, partial string, data any) {
	if WantsJSON(req) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := r.templates.ExecuteTemplate(w, partial, data); err != nil {
		log.Printf("template error (partial %s): %v", partial, err)
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func WantsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json") ||
		strings.HasPrefix(r.URL.Path, "/api/")
}

func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return t.Format("Jan 2")
	}
}
