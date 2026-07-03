package tracker

import (
	"context"
	"errors"
	"strings"
)

// ErrUnauthorized is returned when the API rejects the token (HTTP 401).
// For per-user tokens this means the user must reconnect their account.
var ErrUnauthorized = errors.New("unauthorized")

type ctxLangKey struct{}

// ContextWithLocale returns ctx enriched with a Yandex Tracker API language tag
// derived from locale (e.g. "ru", "ru-RU"). Tracker supports "ru" and "en";
// all other locales fall back to "en".
func ContextWithLocale(ctx context.Context, locale string) context.Context {
	return context.WithValue(ctx, ctxLangKey{}, localeToAPILang(locale))
}

// APILangFromContext returns the Tracker API language stored in ctx, defaulting to "en".
func APILangFromContext(ctx context.Context) string {
	if lang, ok := ctx.Value(ctxLangKey{}).(string); ok && lang != "" {
		return lang
	}
	return "en"
}

// localeToAPILang maps a Mattermost locale string to a Tracker API language tag.
func localeToAPILang(locale string) string {
	if len(locale) >= 2 && strings.EqualFold(locale[:2], "ru") {
		return "ru"
	}
	return "en"
}

type Transition struct {
	ID   string // opaque ID used when executing the transition
	Name string // human-readable label, e.g. "Start Progress"
}

// Client is the interface every tracker integration must implement.
// All methods accept a context so in-flight HTTP requests can be cancelled
// when the plugin shuts down (prevents SIGSEGV in goroutines that outlive deactivation).
type Client interface {
	GetIssue(ctx context.Context, key string) (*Issue, error)
	Ping(ctx context.Context) error // verifies credentials; nil = success
	GetTransitions(ctx context.Context, key string) ([]Transition, error)
	// ExecuteTransition applies the named transition to an issue.
	// fields contains any additional values to include in the request body.
	// Values may be plain strings (free-text fields) or map[string]interface{} (object fields like resolution).
	// Pass nil when no extra fields are needed.
	ExecuteTransition(ctx context.Context, key, transitionID string, fields map[string]interface{}) error
	AssignIssue(ctx context.Context, key, login string) error
	AddComment(ctx context.Context, key, text string) error
	// Myself returns the login of the user who owns the client's token.
	Myself(ctx context.Context) (string, error)
}

// Issue is the canonical domain type used throughout the plugin.
// Tracker-specific API responses are mapped to this struct before
// leaving the tracker package.
type Issue struct {
	Key      string
	Summary  string
	Status   string
	Assignee string // display name; empty string if unassigned
	Priority string
	Type     string
	URL      string
}
