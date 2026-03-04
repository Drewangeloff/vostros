package handler

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/drewangeloff/vostros/internal/auth"
	"github.com/drewangeloff/vostros/internal/ctxutil"
	"github.com/drewangeloff/vostros/internal/model"
	"github.com/drewangeloff/vostros/internal/tmpl"
)

var usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]{3,20}$`)

func (h *Handler) ShowRegister(w http.ResponseWriter, r *http.Request) {
	if ctxutil.GetUser(r.Context()) != nil {
		http.Redirect(w, r, "/timeline", http.StatusSeeOther)
		return
	}
	h.Renderer.Render(w, r, "register.html", tmpl.PageData{Title: "Sign Up"})
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	if tmpl.WantsJSON(r) {
		h.registerJSON(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.Renderer.Render(w, r, "register.html", tmpl.PageData{Title: "Sign Up", Flash: "Invalid form data", FlashType: "error"})
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")

	if errMsg := validateRegistration(username, email, password); errMsg != "" {
		h.Renderer.Render(w, r, "register.html", tmpl.PageData{Title: "Sign Up", Flash: errMsg, FlashType: "error"})
		return
	}

	// Check uniqueness (use identical message to prevent user enumeration)
	existing, err := h.Repo.GetUserByUsername(r.Context(), username)
	if err != nil {
		h.Renderer.Render(w, r, "register.html", tmpl.PageData{Title: "Sign Up", Flash: "Internal error", FlashType: "error"})
		return
	}
	if existing != nil {
		h.Renderer.Render(w, r, "register.html", tmpl.PageData{Title: "Sign Up", Flash: "Username or email already taken", FlashType: "error"})
		return
	}
	existing, err = h.Repo.GetUserByEmail(r.Context(), email)
	if err != nil {
		h.Renderer.Render(w, r, "register.html", tmpl.PageData{Title: "Sign Up", Flash: "Internal error", FlashType: "error"})
		return
	}
	if existing != nil {
		h.Renderer.Render(w, r, "register.html", tmpl.PageData{Title: "Sign Up", Flash: "Username or email already taken", FlashType: "error"})
		return
	}

	hashedPassword, err := auth.HashPassword(password)
	if err != nil {
		h.Renderer.Render(w, r, "register.html", tmpl.PageData{Title: "Sign Up", Flash: "Internal error", FlashType: "error"})
		return
	}

	now := time.Now()
	user := &model.User{
		ID:          auth.NewULID(),
		Username:    username,
		Email:       email,
		Password:    hashedPassword,
		DisplayName: username,
		Role:        "user",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := h.Repo.CreateUser(r.Context(), user); err != nil {
		h.Renderer.Render(w, r, "register.html", tmpl.PageData{Title: "Sign Up", Flash: "Failed to create account", FlashType: "error"})
		return
	}

	// Auto-login
	if err := h.setAuthCookies(w, user); err != nil {
		h.Renderer.Render(w, r, "register.html", tmpl.PageData{Title: "Sign Up", Flash: "Internal error", FlashType: "error"})
		return
	}
	http.Redirect(w, r, "/timeline", http.StatusSeeOther)
}

func (h *Handler) registerJSON(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		h.jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if errMsg := validateRegistration(input.Username, input.Email, input.Password); errMsg != "" {
		h.jsonError(w, errMsg, http.StatusBadRequest)
		return
	}

	existing, err := h.Repo.GetUserByUsername(r.Context(), input.Username)
	if err != nil {
		h.jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if existing != nil {
		h.jsonError(w, "username or email already taken", http.StatusConflict)
		return
	}
	existing, err = h.Repo.GetUserByEmail(r.Context(), input.Email)
	if err != nil {
		h.jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if existing != nil {
		h.jsonError(w, "username or email already taken", http.StatusConflict)
		return
	}

	hashedPassword, err := auth.HashPassword(input.Password)
	if err != nil {
		h.jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	now := time.Now()
	user := &model.User{
		ID:          auth.NewULID(),
		Username:    input.Username,
		Email:       input.Email,
		Password:    hashedPassword,
		DisplayName: input.Username,
		Role:        "user",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := h.Repo.CreateUser(r.Context(), user); err != nil {
		h.jsonError(w, "failed to create account", http.StatusInternalServerError)
		return
	}

	accessToken, err := h.Auth.CreateAccessToken(user.ID, user.Username, user.Role)
	if err != nil {
		h.jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	refreshToken, err := auth.GenerateRefreshToken()
	if err != nil {
		h.jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := h.Repo.CreateRefreshToken(r.Context(), &model.RefreshToken{
		ID:        refreshToken,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
	}); err != nil {
		h.jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{
		"user":          user.OwnProfile(),
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"expires_in":    900,
	})
}

func (h *Handler) ShowLogin(w http.ResponseWriter, r *http.Request) {
	if ctxutil.GetUser(r.Context()) != nil {
		http.Redirect(w, r, "/timeline", http.StatusSeeOther)
		return
	}
	h.Renderer.Render(w, r, "login.html", tmpl.PageData{Title: "Login"})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if tmpl.WantsJSON(r) {
		h.loginJSON(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.Renderer.Render(w, r, "login.html", tmpl.PageData{Title: "Login", Flash: "Invalid form data", FlashType: "error"})
		return
	}

	login := strings.TrimSpace(r.FormValue("login"))
	password := r.FormValue("password")

	user, err := h.findUser(r, login)
	if err != nil {
		h.Renderer.Render(w, r, "login.html", tmpl.PageData{Title: "Login", Flash: "Internal error", FlashType: "error"})
		return
	}
	if user == nil {
		h.Renderer.Render(w, r, "login.html", tmpl.PageData{Title: "Login", Flash: "Invalid credentials", FlashType: "error"})
		return
	}

	valid, _ := auth.VerifyPassword(password, user.Password)
	if !valid {
		h.Renderer.Render(w, r, "login.html", tmpl.PageData{Title: "Login", Flash: "Invalid credentials", FlashType: "error"})
		return
	}

	if user.Role == "banned" {
		h.Renderer.Render(w, r, "login.html", tmpl.PageData{Title: "Login", Flash: "Account suspended", FlashType: "error"})
		return
	}

	if err := h.setAuthCookies(w, user); err != nil {
		h.Renderer.Render(w, r, "login.html", tmpl.PageData{Title: "Login", Flash: "Internal error", FlashType: "error"})
		return
	}
	http.Redirect(w, r, "/timeline", http.StatusSeeOther)
}

func (h *Handler) loginJSON(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		h.jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	user, err := h.findUser(r, input.Login)
	if err != nil {
		h.jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if user == nil {
		h.jsonError(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	valid, _ := auth.VerifyPassword(input.Password, user.Password)
	if !valid {
		h.jsonError(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	if user.Role == "banned" {
		h.jsonError(w, "account suspended", http.StatusForbidden)
		return
	}

	accessToken, err := h.Auth.CreateAccessToken(user.ID, user.Username, user.Role)
	if err != nil {
		h.jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	refreshToken, err := auth.GenerateRefreshToken()
	if err != nil {
		h.jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := h.Repo.CreateRefreshToken(r.Context(), &model.RefreshToken{
		ID:        refreshToken,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
	}); err != nil {
		h.jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"user":          user.OwnProfile(),
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"expires_in":    900,
	})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	if tmpl.WantsJSON(r) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	var input struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.RefreshToken == "" {
		h.jsonError(w, "refresh_token required", http.StatusBadRequest)
		return
	}

	token, err := h.Repo.GetRefreshToken(r.Context(), input.RefreshToken)
	if err != nil || token == nil {
		h.jsonError(w, "invalid refresh token", http.StatusUnauthorized)
		return
	}

	if time.Now().After(token.ExpiresAt) {
		_ = h.Repo.DeleteRefreshToken(r.Context(), token.ID)
		h.jsonError(w, "refresh token expired", http.StatusUnauthorized)
		return
	}

	user, err := h.Repo.GetUserByID(r.Context(), token.UserID)
	if err != nil || user == nil {
		h.jsonError(w, "user not found", http.StatusUnauthorized)
		return
	}

	if user.Role == "banned" {
		_ = h.Repo.DeleteRefreshTokensByUser(r.Context(), user.ID)
		h.jsonError(w, "account suspended", http.StatusForbidden)
		return
	}

	// Rotate: delete old refresh token, issue new pair
	_ = h.Repo.DeleteRefreshToken(r.Context(), token.ID)

	accessToken, err := h.Auth.CreateAccessToken(user.ID, user.Username, user.Role)
	if err != nil {
		h.jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	newRefresh, err := auth.GenerateRefreshToken()
	if err != nil {
		h.jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := h.Repo.CreateRefreshToken(r.Context(), &model.RefreshToken{
		ID:        newRefresh,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
	}); err != nil {
		h.jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"access_token":  accessToken,
		"refresh_token": newRefresh,
		"expires_in":    900,
	})
}

// --- helpers ---

func (h *Handler) findUser(r *http.Request, login string) (*model.User, error) {
	if strings.Contains(login, "@") {
		return h.Repo.GetUserByEmail(r.Context(), login)
	}
	return h.Repo.GetUserByUsername(r.Context(), login)
}

func (h *Handler) setAuthCookies(w http.ResponseWriter, user *model.User) error {
	accessToken, err := h.Auth.CreateAccessToken(user.ID, user.Username, user.Role)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    accessToken,
		Path:     "/",
		MaxAge:   900, // 15 min
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func validateRegistration(username, email, password string) string {
	if !usernameRegex.MatchString(username) {
		return "Username must be 3-20 characters (letters, numbers, underscores)"
	}
	// Basic email validation: must have exactly one @, non-empty local/domain parts, and a dot in domain
	atIdx := strings.IndexByte(email, '@')
	if atIdx < 1 || atIdx != strings.LastIndexByte(email, '@') {
		return "Invalid email address"
	}
	domain := email[atIdx+1:]
	if len(domain) < 3 || !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return "Invalid email address"
	}
	if len(password) < 8 {
		return "Password must be at least 8 characters"
	}
	return ""
}
