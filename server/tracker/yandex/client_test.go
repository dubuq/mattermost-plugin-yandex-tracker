package yandex

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetIssue_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Assert correct path and headers.
		if r.URL.Path != "/issues/PROJECT-1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "OAuth test-token" {
			t.Errorf("unexpected Authorization header: %s", got)
		}
		if got := r.Header.Get("X-Cloud-Org-Id"); got != "test-org" {
			t.Errorf("unexpected X-Cloud-Org-Id header: %s", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"key":      "PROJECT-1",
			"summary":  "Fix the login page crash",
			"status":   map[string]string{"key": "inProgress", "display": "In Progress"},
			"assignee": map[string]string{"display": "Jane Smith"},
			"priority": map[string]string{"key": "critical", "display": "Critical"},
			"type":     map[string]string{"key": "bug", "display": "Bug"},
		})
	}))
	defer srv.Close()

	client := NewWithBase("test-token", "test-org", srv.URL)
	issue, err := client.GetIssue(context.Background(), "PROJECT-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	checks := []struct {
		field string
		got   string
		want  string
	}{
		{"Key", issue.Key, "PROJECT-1"},
		{"Summary", issue.Summary, "Fix the login page crash"},
		{"Status", issue.Status, "In Progress"},
		{"Assignee", issue.Assignee, "Jane Smith"},
		{"Priority", issue.Priority, "Critical"},
		{"Type", issue.Type, "Bug"},
		{"URL", issue.URL, "https://tracker.yandex.ru/PROJECT-1"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: got %q, want %q", c.field, c.got, c.want)
		}
	}
}

func TestGetIssue_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewWithBase("test-token", "test-org", srv.URL)
	_, err := client.GetIssue(context.Background(), "MISSING-99")
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
}

func TestGetIssue_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewWithBase("test-token", "test-org", srv.URL)
	_, err := client.GetIssue(context.Background(), "PROJECT-1")
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
}

func TestGetIssue_UnassignedIssue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"key":      "PROJECT-2",
			"summary":  "Unassigned task",
			"status":   map[string]string{"display": "Open"},
			"priority": map[string]string{"display": "Normal"},
			"type":     map[string]string{"display": "Task"},
			// assignee omitted — should map to empty string, not panic
		})
	}))
	defer srv.Close()

	client := NewWithBase("test-token", "test-org", srv.URL)
	issue, err := client.GetIssue(context.Background(), "PROJECT-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issue.Assignee != "" {
		t.Errorf("expected empty assignee, got %q", issue.Assignee)
	}
}
