package router_test

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	root "github.com/drewangeloff/vostros"
	"github.com/drewangeloff/vostros/internal/auth"
	"github.com/drewangeloff/vostros/internal/handler"
	"github.com/drewangeloff/vostros/internal/model"
	"github.com/drewangeloff/vostros/internal/ratelimit"
	"github.com/drewangeloff/vostros/internal/repository"
	"github.com/drewangeloff/vostros/internal/router"
	"github.com/drewangeloff/vostros/internal/tmpl"
)

// Embedding the interface makes any unexpected database call fail the test.
type discoveryRepo struct {
	repository.Repository
	post       *model.Post
	tokenOwner string
}

func (r *discoveryRepo) GetGlobalTimeline(context.Context, string, int) ([]*model.Post, string, error) {
	return []*model.Post{}, "", nil
}

func (r *discoveryRepo) GetUserByID(_ context.Context, id string) (*model.User, error) {
	return &model.User{ID: id, Username: "owner", Role: "user"}, nil
}

func (r *discoveryRepo) ListAPITokensByUser(_ context.Context, id string) ([]*model.APIToken, error) {
	r.tokenOwner = id
	return []*model.APIToken{{ID: "private-token-id", Name: "private-token-name", Prefix: "vst_test"}}, nil
}

func (r *discoveryRepo) GetReplies(context.Context, string, string, int) ([]*model.Post, string, error) {
	return []*model.Post{}, "", nil
}

func (r *discoveryRepo) GetPostByID(context.Context, string) (*model.Post, error) {
	return r.post, nil
}

func (r *discoveryRepo) CreatePostWithOutbox(_ context.Context, p *model.Post) error {
	r.post = p
	return nil
}

func (r *discoveryRepo) GetUserByUsername(context.Context, string) (*model.User, error) {
	return &model.User{ID: "author", Username: "author", Email: "private@example.com"}, nil
}

func (r *discoveryRepo) GetUserStats(context.Context, string) (*model.UserStats, error) {
	return &model.UserStats{UserID: "author", PostCount: 1}, nil
}

func (r *discoveryRepo) GetPostsByUserID(context.Context, string, string, int) ([]*model.Post, string, error) {
	return []*model.Post{r.post}, "", nil
}

func (r *discoveryRepo) SearchPosts(context.Context, string, string, int) ([]*model.Post, string, error) {
	return []*model.Post{r.post}, "", nil
}

func (r *discoveryRepo) SearchUsers(context.Context, string, int) ([]*model.User, error) {
	return []*model.User{}, nil
}

func TestLegacyResponseFieldsStayCompatible(t *testing.T) {
	repo := &discoveryRepo{post: &model.Post{ID: "post-1", Content: "Useful result"}}
	app, _ := testApp(repo)
	for _, tc := range []struct{ path, old, current string }{
		{"/api/v1/search?q=result", "tweets", "posts"},
		{"/api/v1/users/author", "Tweets", "Posts"},
	} {
		w := request(app, "GET", tc.path, "", "")
		var response map[string]json.RawMessage
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &response) != nil {
			t.Fatalf("invalid response: %s", w.Body.String())
		}
		if string(response[tc.old]) != string(response[tc.current]) || string(response[tc.old]) == "" {
			t.Fatal("legacy and current post fields differ")
		}
		if strings.Contains(w.Body.String(), "private@example.com") {
			t.Fatal("public profile leaked email")
		}
		if tc.old == "Tweets" {
			// user_id is a string, so inspect the two numeric counters directly.
			var rawStats map[string]json.RawMessage
			if err := json.Unmarshal(response["Stats"], &rawStats); err != nil {
				t.Fatal(err)
			}
			if string(rawStats["tweet_count"]) != "1" || string(rawStats["post_count"]) != "1" {
				t.Fatal("legacy and current counts differ")
			}
		}
	}
}

func testApp(repo repository.Repository) (http.Handler, *auth.Service) {
	a := auth.NewService("test-only-secret")
	h := handler.New(repo, tmpl.New(root.TemplateFS, false), a, nil)
	// Exercise the same middleware as production.
	return router.New(h, root.StaticFS, root.DiscoveryFS, auth.NewMiddleware(a, repo), ratelimit.New(1000, time.Minute)), a
}

func request(app http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	return w
}

func TestDiscoveryResourcesArePublicAndMatchPackagedSources(t *testing.T) {
	app, _ := testApp(nil)
	for path, contentType := range map[string]string{
		"/agents": "text/html", "/developers": "text/html", "/skill.md": "text/markdown",
		"/llms.txt": "text/plain", "/robots.txt": "text/plain", "/openapi.json": "application/json", "/sitemap.xml": "application/xml",
	} {
		t.Run(path, func(t *testing.T) {
			w := request(app, "GET", path, "", "")
			if w.Code != 200 {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			if !strings.HasPrefix(w.Header().Get("Content-Type"), contentType) {
				t.Fatalf("content type %q", w.Header().Get("Content-Type"))
			}
			if w.Header().Get("Link") == "" {
				t.Fatal("missing discovery links")
			}
		})
	}
	skill, _ := root.DiscoveryFS.ReadFile("skill/SKILL.md")
	if got := request(app, "GET", "/skill.md", "", "").Body.String(); got != string(skill) {
		t.Fatal("served skill differs from installable skill")
	}
	if w := request(app, "HEAD", "/skill.md", "", ""); w.Code != 200 || w.Body.Len() != 0 {
		t.Fatalf("HEAD: %d, body %d", w.Code, w.Body.Len())
	}
	if w := request(app, "GET", "/SKILL.md", "", ""); w.Code != 308 || w.Header().Get("Location") != "/skill.md" {
		t.Fatal("uppercase skill alias broken")
	}
	var spec struct {
		OpenAPI string         `json:"openapi"`
		Paths   map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(request(app, "GET", "/openapi.json", "", "").Body.Bytes(), &spec); err != nil || spec.OpenAPI != "3.0.3" || len(spec.Paths) == 0 {
		t.Fatalf("invalid OpenAPI: %v", err)
	}
	var sitemap struct {
		URLs []struct {
			Location string `xml:"loc"`
		} `xml:"url"`
	}
	if err := xml.Unmarshal(request(app, "GET", "/sitemap.xml", "", "").Body.Bytes(), &sitemap); err != nil {
		t.Fatal(err)
	}
	for _, u := range sitemap.URLs {
		if !strings.HasPrefix(u.Location, "https://vostros.net/") {
			t.Fatalf("noncanonical sitemap URL %s", u.Location)
		}
	}
}

func TestPublicDocsKeepTokenManagementPrivate(t *testing.T) {
	repo := &discoveryRepo{}
	app, a := testApp(repo)
	guest := request(app, "GET", "/developers", "", "")
	if strings.Contains(guest.Body.String(), "private-token") || strings.Contains(guest.Body.String(), `class="token-create-form"`) || repo.tokenOwner != "" {
		t.Fatal("guest accessed token management")
	}
	token, _ := a.CreateAccessToken("account-1", "owner", "user")
	owner := request(app, "GET", "/developers", "", token)
	if owner.Code != 200 || repo.tokenOwner != "account-1" || !strings.Contains(owner.Body.String(), "private-token-name") {
		t.Fatal("owner cannot manage own tokens")
	}
	if owner.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("token page can be shared-cached")
	}
	for _, path := range []string{"/developers/tokens", "/developers/tokens/private-token-id"} {
		method := "POST"
		if strings.HasSuffix(path, "private-token-id") {
			method = "DELETE"
		}
		r := httptest.NewRequest(method, path, strings.NewReader(`{"name":"test"}`))
		r.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		if w.Code != 401 || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") || !json.Valid(w.Body.Bytes()) {
			t.Fatalf("unauthenticated token write: %d %s", w.Code, w.Body.String())
		}
	}
	if w := request(app, "POST", "/developers/tokens", `name=test`, ""); w.Code != 303 || w.Header().Get("Location") != "/login" {
		t.Fatal("browser auth redirect broken")
	}
}

func TestPermalinkOnlyShowsVisiblePostsAndEscapesContent(t *testing.T) {
	repo := &discoveryRepo{post: &model.Post{ID: "post-1", UserID: "author", Status: "visible", Content: `<script>alert("x")</script>`, CreatedAt: time.Now(), User: &model.User{ID: "author", Username: "author"}}}
	app, _ := testApp(repo)
	w := request(app, "GET", "/p/post-1", "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `href="https://vostros.net/p/post-1"`) {
		t.Fatal("permalink missing or noncanonical")
	}
	if strings.Contains(w.Body.String(), `<script>alert("x")</script>`) || strings.Contains(w.Body.String(), `hx-delete="/post/post-1"`) {
		t.Fatal("post content unescaped or guest can delete")
	}
	for _, status := range []string{"deleted", "held", "hidden"} {
		repo.post.Status = status
		if w := request(app, "GET", "/p/post-1", "", ""); w.Code != 404 || strings.Contains(w.Body.String(), "alert") {
			t.Fatalf("%s post was exposed", status)
		}
	}
	repo.post = nil
	if w := request(app, "GET", "/p/missing", "", ""); w.Code != 404 {
		t.Fatal("missing post did not return 404")
	}
}

func TestUnicodePostLimitsAndLegacyAPICompatibility(t *testing.T) {
	repo := &discoveryRepo{}
	app, a := testApp(repo)
	token, _ := a.CreateAccessToken("account-1", "owner", "user")
	for _, path := range []string{"/api/v1/posts", "/api/v1/tweets"} {
		for _, tc := range []struct {
			text   string
			status int
		}{{strings.Repeat("界", 256), 201}, {strings.Repeat("😀", 256), 201}, {strings.Repeat("界", 257), 400}, {"   ", 400}} {
			body, _ := json.Marshal(map[string]string{"content": tc.text})
			w := request(app, "POST", path, string(body), token)
			if w.Code != tc.status {
				t.Fatalf("%s got %d want %d: %s", path, w.Code, tc.status, w.Body.String())
			}
		}
		if w := request(app, "POST", path, `{"content":"test"}`, ""); w.Code != 401 {
			t.Fatal("anonymous publication allowed")
		}
	}
}
