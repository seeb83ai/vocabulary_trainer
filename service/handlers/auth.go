package handlers

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"vocabulary_trainer/db"

	"golang.org/x/crypto/bcrypt"
)

const cookieName = "vocab_session"
const sessionTTL = 24 * time.Hour

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

// Middleware rejects unauthenticated requests.
// API requests receive 401 JSON; page requests redirect to /login.
func (a *AuthHandler) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == "/login" || strings.HasPrefix(p, "/api/login") || p == "/api/auth/status" ||
			strings.HasSuffix(p, ".js") || strings.HasSuffix(p, ".css") {
			next.ServeHTTP(w, r)
			return
		}
		if !a.validSession(r) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
				return
			}
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
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

	token := a.mintToken()
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
// Always responds 200; body indicates whether auth is enabled.
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

// mintToken creates a signed token: "timestamp:hmac".
func (a *AuthHandler) mintToken() string {
	ts := fmt.Sprintf("%d", time.Now().Unix())
	sig := a.sign(ts)
	return ts + ":" + sig
}

// validSession checks the session cookie signature and expiry.
func (a *AuthHandler) validSession(r *http.Request) bool {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return false
	}
	parts := strings.SplitN(c.Value, ":", 2)
	if len(parts) != 2 {
		return false
	}
	ts, sig := parts[0], parts[1]
	if a.sign(ts) != sig {
		return false
	}
	var unix int64
	if _, err := fmt.Sscanf(ts, "%d", &unix); err != nil {
		return false
	}
	return time.Since(time.Unix(unix, 0)) < sessionTTL
}

func (a *AuthHandler) sign(data string) string {
	mac := hmac.New(sha256.New, a.secret)
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}
