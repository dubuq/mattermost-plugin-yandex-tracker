package main

import (
	"fmt"
	"regexp"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

// issueKeyPattern matches Yandex Tracker issue keys (e.g. PROJECT-123).
// Word boundaries prevent matching inside URLs or longer identifiers.
var issueKeyPattern = regexp.MustCompile(`\b[A-Z][A-Z0-9]{0,19}-\d+\b`)

// MessageHasBeenPosted detects issue keys in new posts and enqueues card builds.
func (p *Plugin) MessageHasBeenPosted(_ *plugin.Context, post *model.Post) {
	defer func() {
		if r := recover(); r != nil {
			p.API.LogError("MessageHasBeenPosted panicked", "err", fmt.Sprintf("%v", r))
		}
	}()
	if !p.shouldHandlePost(post) {
		return
	}

	keys := extractIssueKeys(post.Message)
	if len(keys) == 0 {
		return
	}

	for _, key := range keys {
		if p.store.IsExpanded(post.Id, key) {
			continue
		}

		// Enqueue without a pre-fetched issue — the worker fetches asynchronously
		// so MessageHasBeenPosted returns immediately and does not block the RPC call.
		p.enqueueJob(updateJob{
			issueKey:  key,
			postID:    post.Id,
			channelID: post.ChannelId,
		})
	}
}

func (p *Plugin) shouldHandlePost(post *model.Post) bool {
	if !p.isConfigured() {
		return false
	}
	// Skip bot posts to avoid processing our own updates in a loop.
	if post.UserId == p.botUserID {
		return false
	}
	// Skip system messages (joins, leaves, header changes, etc.).
	if post.Type != "" {
		return false
	}
	// When MonitorAllChannels is disabled, only handle posts in channels
	// where the bot is a member.
	if !p.getConfiguration().MonitorAllChannels {
		if _, appErr := p.API.GetChannelMember(post.ChannelId, p.botUserID); appErr != nil {
			return false
		}
	}
	return true
}

// extractIssueKeys returns deduplicated issue keys found in text, in order.
func extractIssueKeys(text string) []string {
	matches := issueKeyPattern.FindAllString(text, -1)
	seen := make(map[string]bool, len(matches))
	unique := make([]string, 0, len(matches))
	for _, m := range matches {
		if !seen[m] {
			seen[m] = true
			unique = append(unique, m)
		}
	}
	return unique
}

// ensureProps initialises post.Props if nil, preventing nil map write panics.
func ensureProps(post *model.Post) {
	if post.Props == nil {
		post.Props = make(model.StringInterface)
	}
}
