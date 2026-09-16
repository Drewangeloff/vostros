package router_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	root "github.com/drewangeloff/vostros"
	"github.com/drewangeloff/vostros/internal/auth"
	"github.com/drewangeloff/vostros/internal/handler"
	"github.com/drewangeloff/vostros/internal/model"
	"github.com/drewangeloff/vostros/internal/moderation"
	"github.com/drewangeloff/vostros/internal/ratelimit"
	"github.com/drewangeloff/vostros/internal/repository"
	"github.com/drewangeloff/vostros/internal/router"
	"github.com/drewangeloff/vostros/internal/tmpl"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Opt-in real PostgreSQL coverage. Every run gets an isolated schema; no existing
// application tables or accounts are used. See README for the local command.
func TestConversationsWithPostgres(t *testing.T) {
	dsn := os.Getenv("VOSTROS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set VOSTROS_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "vostros_test_" + strings.ToLower(auth.NewULID())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	// Exercise repeat startup against the same additive migrations.
	for range 2 {
		if err := repository.RunMigrations(ctx, pool, root.MigrationsFS); err != nil {
			t.Fatal(err)
		}
	}
	repo := repository.NewPostgres(pool)
	a := auth.NewService("integration-test-only-secret")
	h := handler.New(repo, tmpl.New(root.TemplateFS, false), a, moderation.NewRegexModerator("blockedfixture"))
	app := router.New(h, root.StaticFS, root.DiscoveryFS, auth.NewMiddleware(a, repo), ratelimit.New(10000, time.Minute))
	tokens := map[string]string{}
	for _, name := range []string{"alice", "bravo", "charlie"} {
		user := &model.User{ID: name, Username: name, Email: name + "@test.invalid", Password: "test-only-unused", DisplayName: name, Role: "user", CreatedAt: time.Now(), UpdatedAt: time.Now()}
		if err := repo.CreateUser(ctx, user); err != nil {
			t.Fatal(err)
		}
		tokens[name], err = a.CreateAccessToken(name, name, "user")
		if err != nil {
			t.Fatal(err)
		}
	}
	call := func(method, path, body, who string, want int) *httptest.ResponseRecorder {
		t.Helper()
		w := request(app, method, path, body, tokens[who])
		if w.Code != want {
			t.Fatalf("%s %s as %s: %d want %d: %s", method, path, who, w.Code, want, w.Body.String())
		}
		return w
	}
	create := func(path, content, kind, who string) model.Post {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"content": content, "kind": kind})
		w := call("POST", path, string(body), who, 201)
		var post model.Post
		if err := json.Unmarshal(w.Body.Bytes(), &post); err != nil {
			t.Fatal(err)
		}
		return post
	}
	readInbox := func(who, suffix string) struct {
		Notifications []model.Notification `json:"notifications"`
		Unread        int                  `json:"unread_count"`
		Next          string               `json:"next_cursor"`
	} {
		t.Helper()
		var result struct {
			Notifications []model.Notification `json:"notifications"`
			Unread        int                  `json:"unread_count"`
			Next          string               `json:"next_cursor"`
		}
		w := call("GET", "/api/v1/notifications"+suffix, "", who, 200)
		if w.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatal("inbox can be cached")
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	for _, entry := range []struct{ method, path, body string }{
		{"GET", "/api/v1/notifications", ""}, {"POST", "/api/v1/notifications/read", `{"ids":["1"]}`},
		{"POST", "/api/v1/posts/missing/replies", `{"content":"answer"}`},
		{"PATCH", "/api/v1/posts/missing/state", `{"state":"answered"}`},
	} {
		call(entry.method, entry.path, entry.body, "", 401)
	}
	question := create("/api/v1/posts", "Can someone reproduce this timestamp bug?", "question", "alice")
	if question.Kind != "question" || question.QuestionState != "open" {
		t.Fatal("question did not start open")
	}
	call("POST", "/api/v1/posts", `{"content":"bad type","kind":"unknown"}`, "alice", 400)
	call("POST", "/api/v1/posts/"+question.ID+"/replies", `{"content":"bad kind","kind":"question"}`, "bravo", 400)
	call("POST", "/api/v1/posts/"+question.ID+"/replies", `{"content":"blockedfixture"}`, "bravo", 403)
	call("POST", "/api/v1/posts/missing/replies", `{"content":"test"}`, "bravo", 404)
	reply := create("/api/v1/posts/"+question.ID+"/replies", "@alice @alice use UTC; @charlie can check it. @bravo agrees.", "post", "bravo")
	if reply.ParentID != question.ID || reply.ThreadID != question.ID {
		t.Fatal("reply lost its thread")
	}
	if n := readInbox("alice", ""); len(n.Notifications) != 1 || n.Notifications[0].Kind != "reply" || n.Unread != 1 || n.Notifications[0].Thread.Content != question.Content {
		t.Fatal("missing/doubled reply notification or context")
	}
	if n := readInbox("charlie", ""); len(n.Notifications) != 1 || n.Notifications[0].Kind != "mention" {
		t.Fatal("mention notification missing")
	}
	if n := readInbox("bravo", ""); n.Unread != 0 {
		t.Fatal("self-notification")
	}
	// Reading does not acknowledge an item. Repeated reads return the same unread data.
	aInbox := readInbox("alice", "")
	nID := aInbox.Notifications[0].ID
	call("POST", "/api/v1/notifications/read", fmt.Sprintf(`{"ids":["%d"]}`, nID), "bravo", 200)
	if n := readInbox("alice", ""); n.Unread != 1 {
		t.Fatal("another user marked an inbox item read")
	}
	call("POST", "/api/v1/notifications/read", fmt.Sprintf(`{"ids":["%d"]}`, nID), "alice", 200)
	if n := readInbox("alice", ""); n.Unread != 0 || len(n.Notifications) != 0 {
		t.Fatal("read item stayed unread")
	}
	if n := readInbox("alice", "?unread=false"); len(n.Notifications) != 1 || n.Notifications[0].ReadAt == nil {
		t.Fatal("read history missing")
	}
	for _, body := range []string{`{"ids":[]}`, `{"ids":["-1"]}`, `{"ids":["oops"]}`, `{"ids":[1]}`} {
		call("POST", "/api/v1/notifications/read", body, "alice", 400)
	}
	call("GET", "/api/v1/notifications?cursor=nope", "", "alice", 400)
	call("GET", "/api/v1/notifications?unread=nope", "", "alice", 400)
	nested := create("/api/v1/posts/"+reply.ID+"/replies", "Confirmed with the reproduction. <script>not executable</script>", "post", "charlie")
	if nested.ThreadID != question.ID || nested.ParentID != reply.ID {
		t.Fatal("nested reply split the conversation")
	}
	if n := readInbox("bravo", ""); n.Unread != 1 {
		t.Fatal("parent author not notified")
	}
	if n := readInbox("alice", ""); n.Unread != 1 {
		t.Fatal("thread author not notified")
	}
	for _, who := range []string{"", "bravo"} {
		w := call("GET", "/p/"+nested.ID, "", who, 200)
		if strings.Contains(w.Body.String(), "<script>not executable</script>") || !strings.Contains(w.Body.String(), "Confirmed with the reproduction.") || !strings.Contains(w.Body.String(), question.Content) {
			t.Fatal("thread lost context or failed escaping")
		}
	}
	w := call("GET", "/inbox", "", "bravo", 200)
	if !strings.Contains(w.Body.String(), "Open conversation") || strings.Contains(w.Body.String(), "<script>not executable</script>") {
		t.Fatal("inbox rendering failed")
	}
	call("GET", "/inbox/count", "", "bravo", 200)
	var replies struct {
		Posts    []model.Post `json:"posts"`
		ThreadID string       `json:"thread_id"`
	}
	w = call("GET", "/api/v1/posts/"+reply.ID+"/replies", "", "", 200)
	json.Unmarshal(w.Body.Bytes(), &replies)
	if len(replies.Posts) != 2 || replies.ThreadID != question.ID {
		t.Fatal("API conversation incomplete")
	}
	w = call("GET", "/api/v1/global", "", "", 200)
	if strings.Contains(w.Body.String(), reply.ID) || !strings.Contains(w.Body.String(), question.ID) {
		t.Fatal("replies flooded global feed")
	}
	call("PATCH", "/api/v1/posts/"+question.ID+"/state", `{"state":"answered"}`, "bravo", 403)
	call("PATCH", "/api/v1/posts/"+question.ID+"/state", `{"state":"bad"}`, "alice", 400)
	for _, state := range []string{"answered", "tested", "open"} {
		call("PATCH", "/api/v1/posts/"+question.ID+"/state", `{"state":"`+state+`"}`, "alice", 200)
		w = call("GET", "/api/v1/questions?state="+state, "", "", 200)
		if !strings.Contains(w.Body.String(), question.ID) {
			t.Fatal("question filter missed state")
		}
		call("GET", "/questions?state="+state, "", "alice", 200)
	}
	ordinary := create("/api/v1/posts", "Standalone post", "post", "alice")
	w = call("GET", "/api/v1/timeline", "", "alice", 200)
	if !strings.Contains(w.Body.String(), ordinary.ID) || strings.Contains(w.Body.String(), nested.ID) {
		t.Fatal("home timeline lost own post or included a reply")
	}
	call("PATCH", "/api/v1/posts/"+ordinary.ID+"/state", `{"state":"answered"}`, "alice", 400)
	// Normal browser form submits and HTMX replies both navigate to the result.
	form := url.Values{"content": {"Browser reply"}}
	req := httptest.NewRequest("POST", "/post/"+question.ID+"/replies", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+tokens["bravo"])
	req.Header.Set("HX-Request", "true")
	w = httptest.NewRecorder()
	app.ServeHTTP(w, req)
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("HX-Redirect"), "/p/") {
		t.Fatal("browser reply did not navigate")
	}
	// Paginate inbox and thread without losing context. Numeric cursors are strings.
	for i := 0; i < 21; i++ {
		create("/api/v1/posts/"+question.ID+"/replies", fmt.Sprintf("Reproduction detail %d", i), "post", "bravo")
	}
	n := readInbox("alice", "")
	if len(n.Notifications) != 20 || n.Next == "" {
		t.Fatal("inbox first page broken")
	}
	n2 := readInbox("alice", "?cursor="+n.Next)
	if len(n2.Notifications) == 0 {
		t.Fatal("inbox second page missing")
	}
	seen := map[int64]bool{}
	for _, v := range n.Notifications {
		seen[v.ID] = true
	}
	for _, v := range n2.Notifications {
		if seen[v.ID] {
			t.Fatal("inbox cursor duplicated an item")
		}
	}
	// Delete a reply through HTMX: refresh the conversation and its counts.
	req = httptest.NewRequest("DELETE", "/post/"+reply.ID, nil)
	req.Header.Set("Authorization", "Bearer "+tokens["bravo"])
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Current-URL", "https://vostros.net/p/"+reply.ID)
	w = httptest.NewRecorder()
	app.ServeHTTP(w, req)
	if w.Code != 200 || w.Header().Get("HX-Redirect") != "/p/"+question.ID {
		t.Fatal("reply deletion did not refresh its conversation")
	}
	call("GET", "/api/v1/posts/"+reply.ID, "", "", 404)
	if n := readInbox("charlie", ""); n.Unread != 0 {
		t.Fatal("deleted reply remained in inbox")
	}
	call("POST", "/api/v1/posts/"+reply.ID+"/replies", `{"content":"late"}`, "alice", 404)
	// Deleting a root makes the whole conversation unavailable, including search,
	// profile counts, direct reply links, and previously generated inbox entries.
	call("DELETE", "/api/v1/posts/"+question.ID, "", "alice", 204)
	call("GET", "/p/"+nested.ID, "", "", 404)
	call("GET", "/api/v1/posts/"+nested.ID, "", "", 404)
	call("POST", "/api/v1/posts/"+nested.ID+"/replies", `{"content":"late"}`, "bravo", 404)
	if n := readInbox("alice", "?unread=false"); len(n.Notifications) != 0 {
		t.Fatal("deleted thread leaked into inbox")
	}
	stats, err := repo.GetUserStats(ctx, "charlie")
	if err != nil || stats.PostCount != 0 {
		t.Fatal("profile counted replies to deleted thread")
	}
	w = call("GET", "/api/v1/search?q=Reproduction", "", "", 200)
	if strings.Contains(w.Body.String(), nested.ID) {
		t.Fatal("search exposed removed conversation")
	}
}
