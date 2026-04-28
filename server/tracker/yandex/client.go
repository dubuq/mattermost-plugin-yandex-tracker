package yandex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// GetIssue fetches a single issue by its key (e.g. "PROJECT-123").
func (c *Client) GetIssue(ctx context.Context, key string) (*tracker.Issue, error) {
	url := fmt.Sprintf("%s/issues/%s", c.apiBase, key)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	// OAuth token prefix; use "Bearer" for IAM tokens instead.
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
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
	url := fmt.Sprintf("%s/issues/%s/transitions", c.apiBase, key)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

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
	url := fmt.Sprintf("%s/issues/%s/transitions/%s/_execute", c.apiBase, key, transitionID)

	// Build request body: always a JSON object. Nil/empty fields → send "{}".
	body := make(map[string]interface{}, len(fields))
	for k, v := range fields {
		body[k] = v
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

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
	url := fmt.Sprintf("%s/issues/%s/comments", c.apiBase, key)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusCreated, http.StatusOK:
		return nil
	case http.StatusNotFound:
		return fmt.Errorf("issue %s not found", key)
	case http.StatusBadRequest:
		return fmt.Errorf("invalid issue key: %s", key)
	case http.StatusForbidden, http.StatusUnauthorized:
		return fmt.Errorf("no permission to comment on %s", key)
	default:
		return fmt.Errorf("tracker API error: HTTP %d", resp.StatusCode)
	}
}

func (c *Client) AssignIssue(ctx context.Context, key, login string) error {
	body, _ := json.Marshal(map[string]string{"assignee": login})
	url := fmt.Sprintf("%s/issues/%s", c.apiBase, key)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tracker API error: HTTP %d — check that the login is correct", resp.StatusCode)
	}
	return nil
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
