package main

import (
	"fmt"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/dubuq/mattermost-plugin-yandex-tracker/server/tracker"
)

// markdownEscaper neutralises the characters Mattermost interprets as markdown
// so externally-controlled text (issue summaries, author/assignee names from
// Tracker or webhooks) renders literally and cannot inject links, images,
// mentions, or table/formatting control characters.
var markdownEscaper = strings.NewReplacer(
	"\\", "\\\\",
	"`", "\\`",
	"*", "\\*",
	"_", "\\_",
	"{", "\\{",
	"}", "\\}",
	"[", "\\[",
	"]", "\\]",
	"(", "\\(",
	")", "\\)",
	"#", "\\#",
	"+", "\\+",
	"-", "\\-",
	".", "\\.",
	"!", "\\!",
	"|", "\\|",
	"~", "\\~",
	">", "\\>",
	"<", "\\<",
	"&", "\\&",
)

// escapeMarkdown returns s with markdown control characters escaped.
func escapeMarkdown(s string) string {
	return markdownEscaper.Replace(s)
}

// StatusColors holds the sidebar colors for each status group. Defaults are in configuration.go.
type StatusColors struct {
	Active    string // In Progress, In Review
	Done      string // Closed, Resolved, Released
	Cancelled string // Cancelled, Won't fix, Duplicate
	Default   string // Open, To Do, and anything unrecognised
}

// Formatter converts tracker domain types into Mattermost post attachments.
type Formatter struct {
	pluginID string
}

func NewFormatter(pluginID string) *Formatter {
	return &Formatter{pluginID: pluginID}
}

const emptyFieldValue = "—"

// BuildAttachment converts an issue into an inline post attachment.
// collapsed=true shows only Status+Assignee; false shows all fields.
// withWriteActions=false omits the Assign/Change Status buttons — used for
// ephemeral (per-user) cards when the viewer has not connected their Tracker
// account. Shared channel cards always include them; the click handlers gate
// per-user and prompt non-connected users to run /tracker connect.
func (f *Formatter) BuildAttachment(issue *tracker.Issue, sidebarColor string, t Translations, collapsed, withWriteActions bool) *model.SlackAttachment {
	status := issue.Status
	if status == "" {
		status = emptyFieldValue
	}
	assignee := issue.Assignee
	if assignee == "" {
		assignee = emptyFieldValue
	} else {
		assignee = escapeMarkdown(assignee)
	}

	fields := []*model.SlackAttachmentField{
		{Title: t.StatusLabel, Value: status, Short: true},
		{Title: t.AssigneeLabel, Value: assignee, Short: true},
	}

	if !collapsed {
		issueType := issue.Type
		if issueType == "" {
			issueType = emptyFieldValue
		}
		priority := issue.Priority
		if priority == "" {
			priority = emptyFieldValue
		}
		fields = append(fields,
			&model.SlackAttachmentField{Title: t.TypeLabel, Value: issueType, Short: true},
			&model.SlackAttachmentField{Title: t.PriorityLabel, Value: priority, Short: true},
		)
	}

	toggleName := t.ExpandButton
	toggleURL := fmt.Sprintf("/plugins/%s/expand", f.pluginID)
	if !collapsed {
		toggleName = t.CollapseButton
		toggleURL = fmt.Sprintf("/plugins/%s/collapse", f.pluginID)
	}

	actions := []*model.PostAction{
		{
			Name: t.DismissButton,
			Type: model.PostActionTypeButton,
			Integration: &model.PostActionIntegration{
				URL:     fmt.Sprintf("/plugins/%s/dismiss", f.pluginID),
				Context: map[string]interface{}{"issue_key": issue.Key},
			},
		},
		{
			Name: toggleName,
			Type: model.PostActionTypeButton,
			Integration: &model.PostActionIntegration{
				URL:     toggleURL,
				Context: map[string]interface{}{"issue_key": issue.Key},
			},
		},
		{
			Name: t.RefreshButton,
			Type: model.PostActionTypeButton,
			Integration: &model.PostActionIntegration{
				URL:     fmt.Sprintf("/plugins/%s/refresh", f.pluginID),
				Context: map[string]interface{}{"issue_key": issue.Key},
			},
		},
	}

	if withWriteActions {
		actions = append(actions,
			&model.PostAction{
				Name: t.AssignToMeButton,
				Type: model.PostActionTypeButton,
				Integration: &model.PostActionIntegration{
					URL:     fmt.Sprintf("/plugins/%s/assign", f.pluginID),
					Context: map[string]interface{}{"issue_key": issue.Key},
				},
			},
			&model.PostAction{
				Name: t.ChangeStatusButton,
				Type: model.PostActionTypeButton,
				Integration: &model.PostActionIntegration{
					URL:     fmt.Sprintf("/plugins/%s/change-status", f.pluginID),
					Context: map[string]interface{}{"issue_key": issue.Key},
				},
			},
		)
	}

	return &model.SlackAttachment{
		Color:     sidebarColor,
		Title:     fmt.Sprintf("%s: %s", issue.Key, escapeMarkdown(issue.Summary)),
		TitleLink: issue.URL,
		Fields:    fields,
		Actions:   actions,
	}
}

