package handlers

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"vocabulary_trainer/db"
)

const defaultGitHubAPIBaseURL = "https://api.github.com"

// maxScreenshotBytes caps the decoded PNG size. Larger screenshots are dropped
// (the issue is still created) rather than rejected.
const maxScreenshotBytes = 3 << 20

// GitHubHandler creates GitHub issues from in-app bug/idea reports. It is an
// optional feature, gated like DeepL/LLM: when Token or Repo is empty, Create
// returns 503 and the frontend hides the report button.
type GitHubHandler struct {
	Store        *db.Store
	Token        string
	Repo         string // "owner/repo"
	Labels       []string
	AssetsBranch string
	APIBaseURL   string // defaults to https://api.github.com; overridable for tests
}

// validIssueCategories maps the accepted category keys to their display labels
// used in the issue title prefix and body.
var validIssueCategories = map[string]string{
	"idea":     "Idea",
	"bug":      "Bug",
	"question": "Question",
	"misc":     "Misc",
}

type issueRequest struct {
	Category    string `json:"category"`
	Title       string `json:"title"`
	Description string `json:"description"`
	PageURL     string `json:"page_url"`
	Meta        struct {
		UserAgent string `json:"user_agent"`
		Viewport  string `json:"viewport"`
		Locale    string `json:"locale"`
		Timestamp string `json:"timestamp"`
	} `json:"meta"`
	ScreenshotPNGB64 string `json:"screenshot_png_b64"`
}

type issueResponse struct {
	IssueURL string `json:"issue_url"`
	Number   int    `json:"number"`
	ReportID string `json:"report_id"`
}

// githubAPIError carries the HTTP status from a non-2xx GitHub response so
// callers can branch on it (e.g. 404 when the assets branch is missing).
type githubAPIError struct {
	status int
	body   string
}

func (e *githubAPIError) Error() string {
	return fmt.Sprintf("github API HTTP %d: %s", e.status, e.body)
}

func (h *GitHubHandler) enabled() bool {
	return h.Token != "" && h.Repo != ""
}

func (h *GitHubHandler) baseURL() string {
	if h.APIBaseURL != "" {
		return strings.TrimRight(h.APIBaseURL, "/")
	}
	return defaultGitHubAPIBaseURL
}

func (h *GitHubHandler) assetsBranch() string {
	if h.AssetsBranch != "" {
		return h.AssetsBranch
	}
	return "issue-assets"
}

// ConfigFlag reports whether issue reporting is configured, so the frontend
// can decide whether to show the floating report button.
func (h *GitHubHandler) ConfigFlag(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": h.enabled()})
}

// Create files a GitHub issue from an in-app report. A per-report UUID is
// embedded in the issue body; the UUID→user mapping is recorded only in the
// private audit log (no email/PII in the public issue).
func (h *GitHubHandler) Create(w http.ResponseWriter, r *http.Request) {
	if !h.enabled() {
		writeError(w, http.StatusServiceUnavailable, "issue reporting not configured")
		return
	}

	var req issueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	req.Category = strings.TrimSpace(strings.ToLower(req.Category))
	req.Title = strings.TrimSpace(req.Title)
	req.Description = strings.TrimSpace(req.Description)

	if _, ok := validIssueCategories[req.Category]; !ok {
		writeError(w, http.StatusBadRequest, "invalid category")
		return
	}
	if req.Title == "" || req.Description == "" {
		writeError(w, http.StatusBadRequest, "title and description are required")
		return
	}

	reportID, err := newReportID()
	if err != nil {
		internalError(w, err)
		return
	}

	// Screenshot upload is best-effort: any failure logs and proceeds without
	// the embedded image rather than failing the whole report.
	var screenshotURL string
	if req.ScreenshotPNGB64 != "" {
		switch png, derr := decodeScreenshot(req.ScreenshotPNGB64); {
		case derr != nil:
			log.Printf("github issue: drop screenshot: %v", derr)
		case len(png) > maxScreenshotBytes:
			log.Printf("github issue: drop oversized screenshot (%d bytes)", len(png))
		default:
			if url, uerr := h.uploadScreenshot(reportID, png); uerr != nil {
				log.Printf("github issue: screenshot upload failed: %v", uerr)
			} else {
				screenshotURL = url
			}
		}
	}

	title := "[" + validIssueCategories[req.Category] + "] " + req.Title
	number, htmlURL, err := h.createIssue(title, buildIssueBody(req, reportID, screenshotURL), h.Labels)
	if err != nil {
		log.Printf("github issue: create failed: %v", err)
		writeError(w, http.StatusBadGateway, "could not create issue")
		return
	}

	if h.Store != nil {
		detail := fmt.Sprintf("report=%s issue=#%d category=%s", reportID, number, req.Category)
		if err := h.Store.RecordAuditLog(r.Context(), UserIDFromContext(r.Context()), db.AuditGitHubIssue, ClientIP(r), detail); err != nil {
			log.Printf("github issue: audit log: %v", err)
		}
	}

	writeJSON(w, http.StatusCreated, issueResponse{IssueURL: htmlURL, Number: number, ReportID: reportID})
}

// newReportID returns a random RFC-4122 v4 UUID string using crypto/rand.
func newReportID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate report id: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// decodeScreenshot base64-decodes a PNG payload, tolerating a leading data-URL
// prefix (e.g. "data:image/png;base64,...").
func decodeScreenshot(s string) ([]byte, error) {
	if strings.HasPrefix(s, "data:") {
		if i := strings.IndexByte(s, ','); i >= 0 {
			s = s[i+1:]
		}
	}
	return base64.StdEncoding.DecodeString(strings.TrimSpace(s))
}

func buildIssueBody(req issueRequest, reportID, screenshotURL string) string {
	var b strings.Builder
	b.WriteString(req.Description)
	b.WriteString("\n\n---\n\n")
	fmt.Fprintf(&b, "**Category:** %s\n", validIssueCategories[req.Category])
	if req.PageURL != "" {
		fmt.Fprintf(&b, "**Page:** %s\n", sanitizeCell(req.PageURL))
	}
	b.WriteString("\n| Field | Value |\n| --- | --- |\n")
	fmt.Fprintf(&b, "| User agent | %s |\n", sanitizeCell(req.Meta.UserAgent))
	fmt.Fprintf(&b, "| Viewport | %s |\n", sanitizeCell(req.Meta.Viewport))
	fmt.Fprintf(&b, "| Locale | %s |\n", sanitizeCell(req.Meta.Locale))
	fmt.Fprintf(&b, "| Timestamp | %s |\n", sanitizeCell(req.Meta.Timestamp))
	if screenshotURL != "" {
		fmt.Fprintf(&b, "\n![screenshot](%s)\n", screenshotURL)
	}
	fmt.Fprintf(&b, "\n_Report ID: %s_\n", reportID)
	return b.String()
}

// sanitizeCell collapses newlines and escapes pipes so untrusted client values
// cannot break the markdown table layout.
func sanitizeCell(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", "\\|")
	return strings.TrimSpace(s)
}

func (h *GitHubHandler) createIssue(title, body string, labels []string) (int, string, error) {
	payload := map[string]any{"title": title, "body": body}
	if len(labels) > 0 {
		payload["labels"] = labels
	}
	var out struct {
		Number  int    `json:"number"`
		HTMLURL string `json:"html_url"`
	}
	if err := h.apiDo(http.MethodPost, "/repos/"+h.Repo+"/issues", payload, &out); err != nil {
		return 0, "", err
	}
	return out.Number, out.HTMLURL, nil
}

// uploadScreenshot uploads the PNG to the assets branch via the Contents API
// and returns the raw download URL. If the branch is missing (404) it is
// created from the default branch head and the upload is retried once.
func (h *GitHubHandler) uploadScreenshot(reportID string, png []byte) (string, error) {
	path := "issue-screenshots/" + reportID + ".png"
	url, err := h.putContents(path, png)
	if err == nil {
		return url, nil
	}
	var apiErr *githubAPIError
	if errors.As(err, &apiErr) && apiErr.status == http.StatusNotFound {
		if cerr := h.ensureAssetsBranch(); cerr != nil {
			return "", cerr
		}
		return h.putContents(path, png)
	}
	return "", err
}

func (h *GitHubHandler) putContents(path string, content []byte) (string, error) {
	payload := map[string]any{
		"message": "Add issue screenshot " + path,
		"content": base64.StdEncoding.EncodeToString(content),
		"branch":  h.assetsBranch(),
	}
	var out struct {
		Content struct {
			DownloadURL string `json:"download_url"`
		} `json:"content"`
	}
	if err := h.apiDo(http.MethodPut, "/repos/"+h.Repo+"/contents/"+path, payload, &out); err != nil {
		return "", err
	}
	if out.Content.DownloadURL == "" {
		return "", fmt.Errorf("contents upload returned no download_url")
	}
	return out.Content.DownloadURL, nil
}

func (h *GitHubHandler) ensureAssetsBranch() error {
	var repo struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := h.apiDo(http.MethodGet, "/repos/"+h.Repo, nil, &repo); err != nil {
		return err
	}
	var ref struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := h.apiDo(http.MethodGet, "/repos/"+h.Repo+"/git/ref/heads/"+repo.DefaultBranch, nil, &ref); err != nil {
		return err
	}
	payload := map[string]any{"ref": "refs/heads/" + h.assetsBranch(), "sha": ref.Object.SHA}
	return h.apiDo(http.MethodPost, "/repos/"+h.Repo+"/git/refs", payload, nil)
}

// apiDo performs an authenticated JSON request against the GitHub API. body and
// out may be nil. Non-2xx responses return a *githubAPIError.
func (h *GitHubHandler) apiDo(method, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		bs, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		rdr = bytes.NewReader(bs)
	}
	req, err := http.NewRequest(method, h.baseURL()+path, rdr)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+h.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail := respBytes
		if len(detail) > 200 {
			detail = detail[:200]
		}
		return &githubAPIError{status: resp.StatusCode, body: string(detail)}
	}
	if out != nil {
		if err := json.Unmarshal(respBytes, out); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}
	}
	return nil
}
