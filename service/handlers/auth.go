package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"vocabulary_trainer/db"

	"golang.org/x/crypto/bcrypt"
)

const cookieName = "vocab_session"
const sessionTTL = 24 * time.Hour

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

// AuthHandler handles login/logout and provides middleware.
type AuthHandler struct {
	store  *db.Store
	secret []byte // HMAC signing key, generated at startup
}

// NewAuthHandler creates an AuthHandler backed by the given store.
func NewAuthHandler(store *db.Store) (*AuthHandler, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("generate auth secret: %w", err)
	}
	return &AuthHandler{store: store, secret: secret}, nil
}

// Middleware rejects unauthenticated requests and injects the user ID into context.
// API requests receive 401 JSON; page requests redirect to /login.
func (a *AuthHandler) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == "/login" || strings.HasPrefix(p, "/api/login") || p == "/api/auth/status" ||
			strings.HasSuffix(p, ".js") || strings.HasSuffix(p, ".css") {
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
			http.Redirect(w, r, "/login", http.StatusFound)
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
// The HMAC is computed over "userID:timestamp" to bind both to the signature.
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
	// Token format: "userID:timestamp:hmac"
	lastColon := strings.LastIndex(c.Value, ":")
	if lastColon < 0 {
		return 0, false
	}
	payload, sig := c.Value[:lastColon], c.Value[lastColon+1:]
	if a.sign(payload) != sig {
		return 0, false
	}
	// payload = "userID:timestamp"
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
