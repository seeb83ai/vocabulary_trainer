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
)

const cookieName = "vocab_session"
const sessionTTL = 24 * time.Hour

// AuthHandler handles login/logout and provides middleware when auth is enabled.
type AuthHandler struct {
	username string
	password string
	secret   []byte // HMAC signing key, generated at startup
}

// NewAuthHandler returns an AuthHandler if both user and password are non-empty,
// otherwise returns nil (auth disabled).
func NewAuthHandler(username, password string) (*AuthHandler, error) {
	if username == "" || password == "" {
		return nil, nil
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("generate auth secret: %w", err)
	}
	return &AuthHandler{username: username, password: password, secret: secret}, nil
}

// Middleware rejects unauthenticated requests.
// API requests receive 401 JSON; page requests redirect to /login.
func (a *AuthHandler) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" || strings.HasPrefix(r.URL.Path, "/api/login") || r.URL.Path == "/api/auth/status" {
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
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
		return
	}
	if req.Username != a.username || req.Password != a.password {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid credentials"})
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
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// AuthStatus returns a handler for GET /api/auth/status.
// Always responds 200; body indicates whether auth is enabled.
func AuthStatus(a *AuthHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"auth": a != nil})
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
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
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
