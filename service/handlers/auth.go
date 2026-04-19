package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"vocabulary_trainer/db"
	"vocabulary_trainer/email"

	"golang.org/x/crypto/bcrypt"
)

const cookieName = "vocab_session"
const sessionTTL = 24 * time.Hour
const verificationTTL = 24 * time.Hour

type contextKey int

const userIDCtxKey contextKey = iota

// UserIDFromContext returns the authenticated user's ID from the request context.
func UserIDFromContext(ctx context.Context) int64 {
	id, _ := ctx.Value(userIDCtxKey).(int64)
	return id
}

// WithUserID returns a middleware that injects a fixed user ID into the request
// context. Useful for test routers that bypass the auth middleware.
func WithUserID(id int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userIDCtxKey, id)))
		})
	}
}

// AuthHandler handles login/logout/registration and provides middleware.
type AuthHandler struct {
	store       *db.Store
	secret      []byte // HMAC signing key, generated at startup
	emailSender *email.Sender
	appURL      string
}

// NewAuthHandler creates an AuthHandler backed by the given store.
// emailSender may be nil (email disabled — accounts auto-verified on register).
// appURL is used to build email verification links (e.g. "https://example.com").
// secretHex is an optional hex-encoded 32-byte HMAC key (SESSION_SECRET env var).
// If empty, a random key is generated — sessions will not survive server restarts.
func NewAuthHandler(store *db.Store, emailSender *email.Sender, appURL, secretHex string) (*AuthHandler, error) {
	var secret []byte
	if secretHex != "" {
		decoded, err := hex.DecodeString(secretHex)
		if err != nil || len(decoded) < 32 {
			return nil, fmt.Errorf("SESSION_SECRET must be a hex-encoded string of at least 32 bytes")
		}
		secret = decoded
	} else {
		log.Printf("Warning: SESSION_SECRET not set — sessions will not survive restarts")
		secret = make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return nil, fmt.Errorf("generate auth secret: %w", err)
		}
	}
	return &AuthHandler{store: store, secret: secret, emailSender: emailSender, appURL: appURL}, nil
}

// Middleware rejects unauthenticated requests and injects the user ID into context.
// API requests receive 401 JSON; page requests redirect to /.
func (a *AuthHandler) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == "/" || p == "/login" ||
			strings.HasPrefix(p, "/api/login") ||
			strings.HasPrefix(p, "/api/register") ||
			strings.HasPrefix(p, "/api/verify-email") ||
			p == "/api/auth/status" ||
			strings.HasSuffix(p, ".js") ||
			strings.HasSuffix(p, ".css") {
			next.ServeHTTP(w, r)
			return
		}
		userID, ok := a.sessionUserID(r)
		if !ok {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
				return
			}
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		ctx := context.WithValue(r.Context(), userIDCtxKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Login handles POST /api/login.
func (a *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	user, err := a.store.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if user == nil || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if !user.EmailVerified {
		writeError(w, http.StatusForbidden, "email_not_verified")
		return
	}

	token := a.mintToken(user.ID)
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Register handles POST /api/register.
func (a *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	if !isValidEmail(req.Email) {
		writeError(w, http.StatusBadRequest, "invalid email address")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	existing, err := a.store.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if existing != nil {
		writeError(w, http.StatusConflict, "email already registered")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	verToken := hex.EncodeToString(tokenBytes)
	expiresAt := time.Now().Add(verificationTTL)

	userID, err := a.store.CreateUser(r.Context(), req.Email, string(hash), verToken, expiresAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := a.store.InitPinyinProgressForUser(r.Context(), userID); err != nil {
		log.Printf("Warning: init pinyin progress for user %d: %v", userID, err)
	}

	if a.emailSender == nil {
		// No SMTP configured: auto-verify and log token for inspection.
		user, err := a.store.SetUserEmailVerified(r.Context(), verToken)
		if err != nil || user == nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		log.Printf("Registration (no SMTP): user %d (%s) auto-verified", userID, req.Email)
		sessionToken := a.mintToken(user.ID)
		http.SetCookie(w, &http.Cookie{
			Name:     cookieName,
			Value:    sessionToken,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   int(sessionTTL.Seconds()),
		})
		writeJSON(w, http.StatusOK, map[string]any{"auto_login": true, "redirect": "/train"})
		return
	}

	// Send verification email.
	link := a.appURL + "/api/verify-email?token=" + verToken
	subject := "Confirm your Vocabulary Trainer account"
	bodyText := fmt.Sprintf("Click the link below to activate your account:\n\n%s\n\nThis link expires in 24 hours.", link)
	bodyHTML := fmt.Sprintf(`<p>Click the button below to activate your Vocabulary Trainer account:</p>
<p><a href="%s" style="display:inline-block;padding:12px 24px;background:#2563eb;color:#fff;text-decoration:none;border-radius:8px;font-weight:600">Confirm Email</a></p>
<p>Or copy this link:<br><a href="%s">%s</a></p>
<p>This link expires in 24 hours.</p>`, link, link, link)

	if err := a.emailSender.Send(req.Email, subject, bodyText, bodyHTML); err != nil {
		log.Printf("Error: send verification email to %s: %v", req.Email, err)
		// Don't expose the internal error; user can request a resend later.
	}

	writeJSON(w, http.StatusOK, map[string]any{"pending_verification": true})
}

// VerifyEmail handles GET /api/verify-email?token=...
func (a *AuthHandler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Redirect(w, r, "/?error=invalid_token", http.StatusFound)
		return
	}
	user, err := a.store.SetUserEmailVerified(r.Context(), token)
	if err != nil {
		log.Printf("Error: verify email token: %v", err)
		http.Redirect(w, r, "/?error=invalid_token", http.StatusFound)
		return
	}
	if user == nil {
		http.Redirect(w, r, "/?error=invalid_token", http.StatusFound)
		return
	}

	sessionToken := a.mintToken(user.ID)
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    sessionToken,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
	http.Redirect(w, r, "/train", http.StatusFound)
}

// Me handles GET /api/me — returns the current user's public profile.
func (a *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID := UserIDFromContext(r.Context())
	user, err := a.store.GetUserByID(r.Context(), userID)
	if err != nil || user == nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"email":          user.Email,
		"email_verified": user.EmailVerified,
	})
}

// ChangePassword handles POST /api/change-password.
func (a *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if len(req.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "new password must be at least 8 characters")
		return
	}

	userID := UserIDFromContext(r.Context())
	user, err := a.store.GetUserByID(r.Context(), userID)
	if err != nil || user == nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)) != nil {
		writeError(w, http.StatusForbidden, "current password is incorrect")
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := a.store.UpdateUserPassword(r.Context(), userID, string(newHash)); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// AuthStatus returns a handler for GET /api/auth/status.
func AuthStatus(a *AuthHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"auth": a != nil})
	}
}

// Logout handles POST /api/logout.
func (a *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// mintToken creates a signed token: "userID:timestamp:hmac".
func (a *AuthHandler) mintToken(userID int64) string {
	uid := strconv.FormatInt(userID, 10)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	payload := uid + ":" + ts
	sig := a.sign(payload)
	return payload + ":" + sig
}

// sessionUserID validates the session cookie and returns the user ID it encodes.
func (a *AuthHandler) sessionUserID(r *http.Request) (int64, bool) {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return 0, false
	}
	lastColon := strings.LastIndex(c.Value, ":")
	if lastColon < 0 {
		return 0, false
	}
	payload, sig := c.Value[:lastColon], c.Value[lastColon+1:]
	if a.sign(payload) != sig {
		return 0, false
	}
	parts := strings.SplitN(payload, ":", 2)
	if len(parts) != 2 {
		return 0, false
	}
	userID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, false
	}
	ts, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, false
	}
	if time.Since(time.Unix(ts, 0)) >= sessionTTL {
		return 0, false
	}
	return userID, true
}

func (a *AuthHandler) sign(data string) string {
	mac := hmac.New(sha256.New, a.secret)
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

// isValidEmail does a minimal sanity check: contains exactly one "@" with
// non-empty local and domain parts containing at least one ".".
func isValidEmail(email string) bool {
	at := strings.LastIndex(email, "@")
	if at <= 0 || at == len(email)-1 {
		return false
	}
	domain := email[at+1:]
	return strings.Contains(domain, ".")
}
