package yandex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dubuq/mattermost-plugin-yandex-tracker/server/tracker"
)

const defaultAPIBase = "https://api.tracker.yandex.net/v2"

type Client struct {
	token   string
	orgID   string
	apiBase string
	httpClient *http.Client
}

func New(token, orgID string) *Client {
	return &Client{
		token:   token,
		orgID:   orgID,
		apiBase: defaultAPIBase,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// NewWithBase creates a Client pointed at a custom API base URL.
// Used in tests and for local development against a mock server.
func NewWithBase(token, orgID, apiBase string) *Client {
	c := New(token, orgID)
	c.apiBase = apiBase
	return c
}

// Ping verifies credentials via /myself. 401/403 = bad token; network error = unreachable.
func (c *Client) Ping(ctx context.Context) error {
	url := c.apiBase + "/myself"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "OAuth "+c.token)
	req.Header.Set("X-Cloud-Org-Id", c.orgID)
	req.Header.Set("X-Org-Id", c.orgID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("invalid credentials (HTTP %d) — check your token and org ID", resp.StatusCode)
	default:
		return fmt.Errorf("tracker API returned HTTP %d", resp.StatusCode)
	}
}

// Myself returns the Tracker login of the token owner via GET /myself.
func (c *Client) Myself(ctx context.Context) (string, error) {
	resp, err := c.send(ctx, http.MethodGet, c.apiBase+"/myself", nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return "", fmt.Errorf("myself: %w", tracker.ErrUnauthorized)
	}
	if resp.StatusCode != http.StatusOK {
		// Include the response body: Tracker's 403s explain the actual cause
		// (no org access, missing scope, …), which is critical when diagnosing
		// per-user connection failures.
		rawBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("tracker API error: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(rawBody)))
	}

	var raw apiMyself
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if raw.Login == "" {
		return "", fmt.Errorf("myself: response has no login")
	}
	return raw.Login, nil
}

// GetIssue fetches a single issue by its key (e.g. "PROJECT-123").
func (c *Client) GetIssue(ctx context.Context, key string) (*tracker.Issue, error) {
	url := fmt.Sprintf("%s/issues/%s", c.apiBase, url.PathEscape(key))

	resp, err := c.send(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("issue not found: %s", key)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tracker API error: HTTP %d", resp.StatusCode)
	}

	var raw apiIssue
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return raw.toIssue(), nil
}

func (c *Client) GetTransitions(ctx context.Context, key string) ([]tracker.Transition, error) {
	reqURL := fmt.Sprintf("%s/issues/%s/transitions", c.apiBase, url.PathEscape(key))
	resp, err := c.send(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("get transitions: %w", tracker.ErrUnauthorized)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tracker API error: HTTP %d", resp.StatusCode)
	}

	var raw []apiTransition
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	transitions := make([]tracker.Transition, 0, len(raw))
	for _, t := range raw {
		name := t.Display
		if name == "" {
			name = t.To.Display
		}
		transitions = append(transitions, tracker.Transition{ID: t.ID, Name: name})
	}
	return transitions, nil
}

func (c *Client) ExecuteTransition(ctx context.Context, key, transitionID string, fields map[string]interface{}) error {
	reqURL := fmt.Sprintf("%s/issues/%s/transitions/%s/_execute", c.apiBase, url.PathEscape(key), url.PathEscape(transitionID))

	// Build request body: always a JSON object. Nil/empty fields → send "{}".
	body := make(map[string]interface{}, len(fields))
	for k, v := range fields {
		body[k] = v
	}
	bodyBytes, _ := json.Marshal(body)

	resp, err := c.send(ctx, http.MethodPost, reqURL, bodyBytes)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("execute transition: %w", tracker.ErrUnauthorized)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		rawBody, _ := io.ReadAll(resp.Body)
		var apiErr struct {
			ErrorMessages []string `json:"errorMessages"`
		}
		if jsonErr := json.Unmarshal(rawBody, &apiErr); jsonErr == nil && len(apiErr.ErrorMessages) > 0 {
			return fmt.Errorf("%s", strings.Join(apiErr.ErrorMessages, "; "))
		}
		return fmt.Errorf("tracker API error: HTTP %d", resp.StatusCode)
	}
	return nil
}

// AddComment posts a comment on an issue.
func (c *Client) AddComment(ctx context.Context, key, text string) error {
	body, _ := json.Marshal(map[string]string{"text": text})
	reqURL := fmt.Sprintf("%s/issues/%s/comments", c.apiBase, url.PathEscape(key))
	resp, err := c.send(ctx, http.MethodPost, reqURL, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusCreated, http.StatusOK:
		return nil
	case http.StatusNotFound:
		return fmt.Errorf("issue %s not found", key)
	case http.StatusBadRequest:
		return fmt.Errorf("invalid issue key: %s", key)
	case http.StatusUnauthorized:
		return fmt.Errorf("add comment: %w", tracker.ErrUnauthorized)
	case http.StatusForbidden:
		return fmt.Errorf("no permission to comment on %s", key)
	default:
		return fmt.Errorf("tracker API error: HTTP %d", resp.StatusCode)
	}
}

func (c *Client) AssignIssue(ctx context.Context, key, login string) error {
	body, _ := json.Marshal(map[string]string{"assignee": login})
	reqURL := fmt.Sprintf("%s/issues/%s", c.apiBase, url.PathEscape(key))
	resp, err := c.send(ctx, http.MethodPatch, reqURL, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("assign: %w", tracker.ErrUnauthorized)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tracker API error: HTTP %d — check that the login is correct", resp.StatusCode)
	}
	return nil
}

// maxRetries bounds transient-failure retries so a struggling Tracker API can
// never turn into an unbounded retry loop.
const maxRetries = 2

// send performs an HTTP request with the client's standard headers and retries
// on transient failures — network errors, HTTP 429, and 5xx — using bounded
// exponential backoff that honours a Retry-After header when present. body may
// be nil (GET); it is re-read from the buffer on each attempt. Non-transient
// responses (including 4xx like 401/403/404) are returned to the caller as-is.
func (c *Client) send(ctx context.Context, method, reqURL string, body []byte) (*http.Response, error) {
	backoff := 500 * time.Millisecond
	var lastErr error
	for attempt := 0; ; attempt++ {
		var r io.Reader
		if body != nil {
			r = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, reqURL, r)
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
		c.setHeaders(req)

		resp, err := c.httpClient.Do(req)
		switch {
		case err != nil:
			lastErr = fmt.Errorf("request failed: %w", err)
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			wait := retryAfter(resp, backoff)
			resp.Body.Close()
			lastErr = fmt.Errorf("tracker API error: HTTP %d", resp.StatusCode)
			if attempt < maxRetries {
				if !sleepCtx(ctx, wait) {
					return nil, ctx.Err()
				}
				backoff *= 2
				continue
			}
			return nil, lastErr
		default:
			return resp, nil
		}
		// Network error path.
		if attempt >= maxRetries {
			return nil, lastErr
		}
		if !sleepCtx(ctx, backoff) {
			return nil, ctx.Err()
		}
		backoff *= 2
	}
}

// retryAfter returns the delay before the next attempt, preferring the server's
// Retry-After header (seconds) and falling back to the caller's backoff.
func retryAfter(resp *http.Response, fallback time.Duration) time.Duration {
	if v := resp.Header.Get("Retry-After"); v != "" {
		if secs, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && secs > 0 {
			if d := time.Duration(secs) * time.Second; d <= 30*time.Second {
				return d
			}
			return 30 * time.Second
		}
	}
	return fallback
}

// sleepCtx waits for d or until ctx is cancelled. Returns false if cancelled.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// setHeaders applies auth, org ID, and locale headers common to all requests.
// The Accept-Language value is read from the request's context (set via tracker.ContextWithLocale),
// defaulting to "en" when not present.
func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "OAuth "+c.token)
	req.Header.Set("X-Cloud-Org-Id", c.orgID)
	req.Header.Set("X-Org-Id", c.orgID)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Language", tracker.APILangFromContext(req.Context()))
}
