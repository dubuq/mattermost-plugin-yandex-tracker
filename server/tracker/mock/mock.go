package mock

import (
	"context"
	"fmt"

	"github.com/dubuq/mattermost-plugin-yandex-tracker/server/tracker"
)

// Client is a test double for tracker.Client.
// Populate Issues with fixtures; set Err to simulate failures.
type Client struct {
	Issues map[string]*tracker.Issue
	Err    error
}

// Ping always succeeds for the mock — credentials are not checked in tests.
func (m *Client) Ping(_ context.Context) error {
	return m.Err
}

func (m *Client) GetIssue(_ context.Context, key string) (*tracker.Issue, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if issue, ok := m.Issues[key]; ok {
		return issue, nil
	}
	return nil, fmt.Errorf("mock: issue not found: %s", key)
}

func (m *Client) GetTransitions(_ context.Context, _ string) ([]tracker.Transition, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return []tracker.Transition{
		{ID: "open", Name: "Open"},
		{ID: "inProgress", Name: "In Progress"},
		{ID: "closed", Name: "Closed"},
	}, nil
}

// ExecuteTransition is a no-op in tests.
func (m *Client) ExecuteTransition(_ context.Context, _, _ string, _ map[string]interface{}) error {
	return m.Err
}

// AssignIssue is a no-op in tests.
func (m *Client) AssignIssue(_ context.Context, _, _ string) error { return m.Err }

// AddComment is a no-op in tests.
func (m *Client) AddComment(_ context.Context, _, _ string) error { return m.Err }

// Myself returns a fixed login for tests.
func (m *Client) Myself(_ context.Context) (string, error) {
	if m.Err != nil {
		return "", m.Err
	}
	return "mock-user", nil
}
