package handlers

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
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
	"golang.org/x/crypto/pbkdf2"
)

const cookieName = "vocab_session"
const settingsKeyCookie = "vocab_settings_key"
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

// Secret returns the server's HMAC/encryption secret so other handlers can use
// it for sealing the per-user settings key cookie.
func (a *AuthHandler) Secret() []byte { return a.secret }

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
	a.setSettingsKeyCookie(w, r, user.ID, req.Password)
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

	userID, err := a.store.CreateUserWithSettings(r.Context(), req.Email, string(hash), verToken, expiresAt)
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
		a.setSettingsKeyCookie(w, r, user.ID, req.Password)
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

	// Re-encrypt API keys with the new derived key.
	if err := a.reencryptAPIKeys(w, r, userID, req.NewPassword); err != nil {
		log.Printf("Warning: re-encrypt API keys for user %d: %v", userID, err)
		// Non-fatal: keys become inaccessible until user re-saves them.
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
	http.SetCookie(w, &http.Cookie{
		Name:     settingsKeyCookie,
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

// --- Settings-key crypto helpers ---

// deriveSettingsKey runs PBKDF2-SHA256(password, saltHex, 100_000) → 32-byte key.
func deriveSettingsKey(password, saltHex string) ([]byte, error) {
	salt, err := hex.DecodeString(saltHex)
	if err != nil {
		return nil, fmt.Errorf("decode salt: %w", err)
	}
	return pbkdf2.Key([]byte(password), salt, 100_000, 32, sha256.New), nil
}

// sealKey AES-GCM-encrypts a 32-byte derived key with the server secret.
// Returns standard base64.
// SealKey encrypts derivedKey with secret using AES-GCM and returns a base64 string.
// Exported so tests can round-trip without using the auth handler.
func SealKey(secret, derivedKey []byte) (string, error) {
	return sealKey(secret, derivedKey)
}

func sealKey(secret, derivedKey []byte) (string, error) {
	block, err := aes.NewCipher(secret[:32])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nonce, nonce, derivedKey, nil)
	return base64.StdEncoding.EncodeToString(ct), nil
}

// OpenSettingsKey decodes the settings_key cookie value and decrypts it.
// Exported so SettingsHandler and translate/LLM handlers can call it.
func OpenSettingsKey(secret []byte, sealed string) ([]byte, error) {
	ct, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil {
		return nil, fmt.Errorf("decode sealed key: %w", err)
	}
	block, err := aes.NewCipher(secret[:32])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(ct) < ns {
		return nil, fmt.Errorf("ciphertext too short")
	}
	return gcm.Open(nil, ct[:ns], ct[ns:], nil)
}

// EncryptAPIKey AES-GCM-encrypts a plaintext API key with the derived key.
// Returns standard base64. Returns "" for empty plaintext.
func EncryptAPIKey(derivedKey []byte, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ct), nil
}

// DecryptAPIKey is the inverse of EncryptAPIKey. Returns "" for empty ciphertext.
func DecryptAPIKey(derivedKey []byte, ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	ct, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode api key: %w", err)
	}
	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	ns := gcm.NonceSize()
	if len(ct) < ns {
		return "", fmt.Errorf("ciphertext too short")
	}
	pt, err := gcm.Open(nil, ct[:ns], ct[ns:], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt api key: %w", err)
	}
	return string(pt), nil
}

// MaskKey returns a masked display of a key (shows last ≤4 chars).
// Returns "" for empty input so callers can distinguish "no key" from "key set".
func MaskKey(plaintext string) string {
	if plaintext == "" {
		return ""
	}
	suffix := plaintext
	if len(suffix) > 4 {
		suffix = suffix[len(suffix)-4:]
	}
	return "****" + suffix
}

// setSettingsKeyCookie derives the per-user encryption key from the plaintext
// password + their stored salt, seals it with the server secret, and writes a
// HttpOnly cookie.
func (a *AuthHandler) setSettingsKeyCookie(w http.ResponseWriter, r *http.Request, userID int64, password string) {
	_, salt, _, _, err := a.store.GetUserSettingsRaw(r.Context(), userID)
	if err != nil || salt == "" {
		return
	}
	derivedKey, err := deriveSettingsKey(password, salt)
	if err != nil {
		return
	}
	sealed, err := sealKey(a.secret, derivedKey)
	if err != nil {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     settingsKeyCookie,
		Value:    sealed,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
}

// reencryptAPIKeys re-derives the settings key from the new password and
// re-encrypts any stored API keys. Called after a successful password change.
func (a *AuthHandler) reencryptAPIKeys(w http.ResponseWriter, r *http.Request, userID int64, newPassword string) error {
	_, salt, deeplEnc, llmEnc, err := a.store.GetUserSettingsRaw(r.Context(), userID)
	if err != nil {
		return err
	}
	if salt == "" {
		return nil
	}

	// Get old derived key from the existing settings_key cookie.
	oldDerivedKey, cookieErr := func() ([]byte, error) {
		c, err := r.Cookie(settingsKeyCookie)
		if err != nil {
			return nil, err
		}
		return OpenSettingsKey(a.secret, c.Value)
	}()

	newDerivedKey, err := deriveSettingsKey(newPassword, salt)
	if err != nil {
		return err
	}

	// Re-encrypt API keys only when we have the old key to decrypt them.
	if cookieErr == nil && oldDerivedKey != nil {
		if deeplEnc != "" {
			pt, err := DecryptAPIKey(oldDerivedKey, deeplEnc)
			if err == nil {
				deeplEnc, _ = EncryptAPIKey(newDerivedKey, pt)
			}
		}
		if llmEnc != "" {
			pt, err := DecryptAPIKey(oldDerivedKey, llmEnc)
			if err == nil {
				llmEnc, _ = EncryptAPIKey(newDerivedKey, pt)
			}
		}
		// Load current llm_provider and llm_local_url from settings.
		st, _, _, _, _ := a.store.GetUserSettingsRaw(r.Context(), userID)
		provider := ""
		localURL := ""
		if st != nil {
			provider = st.LLMProvider
			localURL = st.LLMLocalURL
		}
		_ = a.store.UpdateUserAPIKeys(r.Context(), userID, deeplEnc, provider, llmEnc, localURL)
	}

	// Always issue the new settings-key cookie.
	sealed, err := sealKey(a.secret, newDerivedKey)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     settingsKeyCookie,
		Value:    sealed,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
	return nil
}
