package handlers

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"log"
	"net/http"
	"strings"
	"time"
)

// DefaultPwnedPasswordsURL is the HIBP Pwned Passwords range API. The first
// five hex characters of the password's SHA-1 hash are appended to it.
const DefaultPwnedPasswordsURL = "https://api.pwnedpasswords.com/range/"

// errPwnedPassword is the error text for a password found in a data breach.
// The browser check in index.js and settings.js uses the same text.
const errPwnedPassword = "This password has appeared in a data breach. Please choose a different password."

// pwnedPasswordClient bounds the HIBP lookup, so a slow API delays a
// registration or password change by at most two seconds.
var pwnedPasswordClient = &http.Client{Timeout: 2 * time.Second}

// isPasswordPwned reports whether password is in the HIBP Pwned Passwords
// list at baseURL. Only the first five characters of the SHA-1 hash leave
// the server (k-anonymity). It fails open: an empty baseURL, a network error,
// a timeout or an error reply return false, so a HIBP outage never blocks a
// user.
func isPasswordPwned(ctx context.Context, baseURL, password string) bool {
	if baseURL == "" {
		return false
	}
	sum := sha1.Sum([]byte(password))
	hash := strings.ToUpper(hex.EncodeToString(sum[:]))
	prefix, suffix := hash[:5], hash[5:]

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+prefix, nil)
	if err != nil {
		log.Printf("HIBP: build request: %v (check skipped)", err)
		return false
	}
	req.Header.Set("Add-Padding", "true")
	resp, err := pwnedPasswordClient.Do(req)
	if err != nil {
		log.Printf("HIBP: %v (check skipped)", err)
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("HIBP: status %d (check skipped)", resp.StatusCode)
		return false
	}
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		hashSuffix, count, ok := strings.Cut(strings.TrimSpace(sc.Text()), ":")
		// Padding lines have a count of 0 and are not real breach entries.
		if ok && strings.EqualFold(hashSuffix, suffix) && count != "0" {
			return true
		}
	}
	return false
}
