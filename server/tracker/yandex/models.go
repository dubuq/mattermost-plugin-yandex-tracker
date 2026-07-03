package yandex

import (
	"fmt"

	"github.com/dubuq/mattermost-plugin-yandex-tracker/server/tracker"
)

const trackerBaseURL = "https://tracker.yandex.ru"

// apiIssue mirrors the Yandex Tracker API response for a single issue.
// Only fields we actually use are declared; the rest are silently ignored.
type apiIssue struct {
	Key      string   `json:"key"`
	Summary  string   `json:"summary"`
	Status   apiField `json:"status"`
	Assignee apiUser  `json:"assignee"`
	Priority apiField `json:"priority"`
	Type     apiField `json:"type"`
}

// apiField is the common shape for enum-like Yandex Tracker fields.
type apiField struct {
	Key     string `json:"key"`
	Display string `json:"display"`
}

// apiUser represents a Yandex user reference in the API response.
type apiUser struct {
	Display string `json:"display"`
}

// apiMyself mirrors the relevant part of GET /v2/myself.
type apiMyself struct {
	Login string `json:"login"`
}

// apiTransition mirrors a single entry from GET /v2/issues/:key/transitions.
type apiTransition struct {
	ID      string   `json:"id"`
	Display string   `json:"display"`
	To      apiField `json:"to"`
}

// toIssue maps the Yandex API response to the shared domain type.
func (a *apiIssue) toIssue() *tracker.Issue {
	return &tracker.Issue{
		Key:      a.Key,
		Summary:  a.Summary,
		Status:   a.Status.Display,
		Assignee: a.Assignee.Display,
		Priority: a.Priority.Display,
		Type:     a.Type.Display,
		URL:      fmt.Sprintf("%s/%s", trackerBaseURL, a.Key),
	}
}
