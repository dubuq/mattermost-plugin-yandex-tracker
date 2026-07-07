package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/dubuq/mattermost-plugin-yandex-tracker/server/tracker"
)

// sourceIP returns the remote host of the request without the port, used as the
// webhook rate-limit key.
func sourceIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// updateJob is a request to rebuild and update one or more post cards for an issue.
type updateJob struct {
	issueKey     string
	issue        *tracker.Issue // pre-fetched (background refresh); nil means fetch fresh
	forceRefresh bool           // skip issue cache; used after mutations so stale 30s cache is not shown
	event        webhookEvent   // non-zero for webhook-driven events (comment, assign, issueCreated)

	// postID and channelID are set only by MessageHasBeenPosted.
	// When non-empty the worker performs SaveIssuePost / MarkExpanded / AddPostIssueKey
	// before updating the card, making the worker the sole owner of post-mapping KV writes.
	postID    string
	channelID string
}

// enqueueUpdate enqueues an issueUpdated job. Safe to call from any goroutine.
func (p *Plugin) enqueueUpdate(issueKey string, issue *tracker.Issue) {
	p.enqueueJob(updateJob{issueKey: issueKey, issue: issue})
}

// enqueueForceUpdate enqueues a job that bypasses the issue cache.
// Use after mutations (transition, assign) and manual refresh so the card
// reflects the actual new state from Tracker, not stale cached data.
func (p *Plugin) enqueueForceUpdate(issueKey string) {
	p.enqueueJob(updateJob{issueKey: issueKey, forceRefresh: true})
}

// enqueueJob enqueues a job onto updateCh. Drops if the plugin is shutting down or the queue is full.
func (p *Plugin) enqueueJob(job updateJob) {
	if p.updateCh == nil {
		// Called before OnActivate completed (e.g. partial init failure) — discard safely.
		return
	}
	select {
	case p.updateCh <- job:
	case <-p.ctx.Done():
		// Plugin is deactivating — silently drop.
	default:
		p.API.LogWarn("enqueueJob: update queue full, dropping job", "key", job.issueKey, "type", job.event.Type)
	}
}

// safeProcessJob wraps processUpdateJob with a last-resort recover so a panic
// in one job does not kill the update worker goroutine.
func (p *Plugin) safeProcessJob(job updateJob) {
	// Skip processing if the plugin is shutting down — the RPC connection may
	// already be in a partially-torn-down state, causing SIGSEGV in chanrecv.
	if p.ctx.Err() != nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			func() {
				defer func() { recover() }()
				p.API.LogError("update worker: job panicked", "key", job.issueKey, "err", fmt.Sprintf("%v", r))
			}()
		}
	}()
	p.processUpdateJob(job)
}

// processUpdateJob dispatches to the appropriate handler based on event type.
// Called only from the single update worker goroutine.
func (p *Plugin) processUpdateJob(job updateJob) {
	// Jobs from MessageHasBeenPosted carry post metadata so the worker can perform
	// all post-mapping KV writes. This must happen before updateIssueCards reads
	// GetIssuePosts / GetPostIssueKeys, otherwise the new post won't be in either list.
	if job.postID != "" && job.issueKey != "" {
		if err := p.store.SaveIssuePost(job.issueKey, job.channelID, job.postID); err != nil {
			p.API.LogError("worker: failed to save issue post", "key", job.issueKey, "postID", job.postID, "err", err.Error())
		} else {
			p.API.LogDebug("worker: ensured post mapping", "key", job.issueKey, "postID", job.postID)
		}
		p.store.AddPostIssueKey(job.postID, job.issueKey)
		if err := p.store.MarkExpanded(job.postID, job.issueKey); err != nil {
			p.API.LogError("worker: failed to mark expanded", "key", job.issueKey, "postID", job.postID, "err", err.Error())
		}
	}

	switch job.event.Type {
	case "commentCreated":
		p.handleCommentNotification(job.event.Key, job.event.Author, job.event.Comment)
	case "issueAssigned":
		p.handleAssignmentNotification(job.event.Key, job.event.Assignee)
	case "issueCreated":
		p.handleIssueCreated(job.event.Key)
	default:
		// "issueUpdated" or empty type (direct enqueueUpdate call) — rebuild cards.
		key := job.issueKey
		if key == "" {
			key = job.event.Key
		}
		if key != "" {
			p.handleStatusChange(key, job.issue, job.forceRefresh)
		}
	}
}

// webhookEvent is the JSON body sent by Yandex Tracker triggers. See README for trigger templates.
type webhookEvent struct {
	Key      string `json:"key"`
	Type     string `json:"type"`
	Comment  string `json:"comment"`  // commentCreated: comment body text
	Author   string `json:"author"`   // commentCreated: display name of commenter
	Assignee string `json:"assignee"` // issueAssigned: display name of new assignee
}

// handleWebhook receives Yandex Tracker events and enqueues card updates.
func (p *Plugin) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Per-source rate limit before any work, so an unauthenticated flood is cheap
	// to shed. Keyed by remote host (strip the port).
	if !p.webhookLimiter.allow(sourceIP(r)) {
		http.Error(w, "too many requests", http.StatusTooManyRequests)
		return
	}

	// 401 on bad secret signals misconfiguration rather than a retriable error.
	if !p.validateWebhookSecret(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var event webhookEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		p.API.LogError("webhook: failed to decode body", "err", err.Error())
		// Still return 200 — a malformed payload won't improve on retry.
		w.WriteHeader(http.StatusOK)
		return
	}

	if !validIssueKey(event.Key) {
		p.API.LogWarn("webhook: received event with missing or malformed issue key")
		w.WriteHeader(http.StatusOK)
		return
	}

	if event.Type == "" {
		p.API.LogDebug("webhook: ignoring event with no type", "key", event.Key)
		w.WriteHeader(http.StatusOK)
		return
	}

	// forceRefresh bypasses the issue cache so the card reflects the new state
	// from Tracker immediately, not stale cached data from before the event.
	p.enqueueJob(updateJob{issueKey: event.Key, event: event, forceRefresh: true})
	w.WriteHeader(http.StatusOK)
}

// handleStatusChange updates cards for issueKey. issue may be pre-fetched (background refresh)
// or nil (webhook/action events), in which case it is fetched from the Tracker API.
// forceRefresh bypasses the issue cache — always set after mutations so the card reflects
// actual new Tracker state rather than stale cached data.
// Must only be called from the single update worker goroutine.
func (p *Plugin) handleStatusChange(issueKey string, issue *tracker.Issue, forceRefresh bool) {
	if issue == nil && !forceRefresh {
		issue = p.store.GetCachedIssue(issueKey)
	}
	if issue == nil {
		client := p.getTrackerClient()
		if client == nil {
			p.API.LogWarn("handleStatusChange: tracker client not configured, skipping", "key", issueKey)
			return
		}
		var err error
		issue, err = client.GetIssue(tracker.ContextWithLocale(p.ctx, p.serverLocale()), issueKey)
		if err != nil {
			p.API.LogError("webhook: failed to fetch issue", "key", issueKey, "err", err.Error())
			return
		}
		p.store.CacheIssue(issueKey, issue)
	}
	p.updateIssueCards(issueKey, issue)
}

// updateIssueCards updates every post card for issueKey.
// Must only be called from the single update worker goroutine.
func (p *Plugin) updateIssueCards(issueKey string, issue *tracker.Issue) {
	issuePosts, err := p.store.GetIssuePosts(issueKey)
	if err != nil {
		p.API.LogError("updateIssueCards: store lookup failed", "key", issueKey, "err", err.Error())
		return
	}
	if len(issuePosts) == 0 {
		p.API.LogDebug("updateIssueCards: no tracked posts for issue, skipping", "key", issueKey)
		return
	}

	cfg := p.getConfiguration()

	allSucceeded := true
	for _, ip := range issuePosts {
		if p.ctx.Err() != nil {
			return
		}
		post, appErr := p.API.GetPost(ip.PostID)
		if appErr != nil {
			errStr := appErr.Error()
			p.API.LogError("updateIssueCards: failed to get post", "postID", ip.PostID, "err", errStr)
			// Remove posts that no longer exist so they stop blocking future updates.
			if strings.Contains(errStr, "not found") || strings.Contains(errStr, "deleted channel") {
				p.store.RemoveIssuePost(issueKey, ip.PostID)
			} else {
				allSucceeded = false
			}
			continue
		}
		updated := p.formatter.BuildAttachment(issue, cfg.resolveStatusColor(issue.Status), p.translations(), true, true)

		// Rebuild the full attachment list from KV — never read from post.Props
		// to avoid crashing on corrupted data written by previous race conditions.
		allKeys := p.store.GetPostIssueKeys(ip.PostID)
		if len(allKeys) == 0 {
			allKeys = []string{issueKey}
		}
		allAttachments := make([]*model.SlackAttachment, 0, len(allKeys))
		for _, k := range allKeys {
			if k == issueKey {
				allAttachments = append(allAttachments, updated)
			} else if a := p.store.GetPostAttachment(ip.PostID, k); a != nil {
				allAttachments = append(allAttachments, a)
			}
		}
		p.store.SetPostAttachment(ip.PostID, issueKey, updated)
		ensureProps(post)
		post.Props["attachments"] = allAttachments
		if _, appErr := p.API.UpdatePost(post); appErr != nil {
			p.API.LogError("updateIssueCards: failed to update post", "postID", post.Id, "err", appErr.Error())
			if strings.Contains(appErr.Error(), "deleted channel") {
				p.store.RemoveIssuePost(issueKey, ip.PostID)
			} else {
				allSucceeded = false
			}
		} else {
			p.API.LogDebug("updateIssueCards: updated post", "key", issueKey, "postID", post.Id, "status", issue.Status)
		}
	}

	// Only advance SetLastStatus when every card updated successfully.
	// If any UpdatePost failed, leave the last status at its old value so the
	// background refresh retries on the next cycle instead of skipping the issue.
	if allSucceeded {
		p.store.SetLastStatus(issueKey, issue.Status)
	}
}

func (p *Plugin) handleIssueCreated(issueKey string) {
	queueKey := queueFromKey(issueKey)
	if queueKey == "" {
		return
	}

	channelIDs, err := p.store.GetSubscribedChannels(queueKey)
	if err != nil {
		p.API.LogError("webhook: failed to get subscriptions", "queue", queueKey, "err", err.Error())
		return
	}
	if len(channelIDs) == 0 {
		return
	}

	client := p.getTrackerClient()
	if client == nil {
		p.API.LogWarn("handleIssueCreated: tracker client not configured, skipping", "key", issueKey)
		return
	}
	issue, err := client.GetIssue(tracker.ContextWithLocale(p.ctx, p.serverLocale()), issueKey)
	if err != nil {
		p.API.LogError("webhook: failed to fetch new issue", "key", issueKey, "err", err.Error())
		return
	}

	cfg := p.getConfiguration()
	attachment := p.formatter.BuildAttachment(issue, cfg.resolveStatusColor(issue.Status), p.translations(), false, true)

	for _, channelID := range channelIDs {
		post := &model.Post{
			UserId:    p.botUserID,
			ChannelId: channelID,
		}
		post.AddProp("attachments", []*model.SlackAttachment{attachment})
		created, appErr := p.API.CreatePost(post)
		if appErr != nil {
			p.API.LogError("webhook: failed to post new issue card", "key", issueKey, "channelID", channelID, "err", appErr.Error())
			continue
		}
		if err := p.store.SaveIssuePost(issueKey, channelID, created.Id); err != nil {
			p.API.LogError("webhook: failed to save issue post", "key", issueKey, "postID", created.Id, "err", err.Error())
		}
		p.store.AddPostIssueKey(created.Id, issueKey)
		p.store.SetPostAttachment(created.Id, issueKey, attachment)
	}
}

// queueFromKey extracts the queue prefix from an issue key, e.g. "DEV" from "DEV-123".
func queueFromKey(issueKey string) string {
	if idx := strings.LastIndex(issueKey, "-"); idx > 0 {
		return issueKey[:idx]
	}
	return ""
}

// handleCommentNotification posts the comment text in each channel where the issue has a card.
// Throttled per issue to absorb Yandex Tracker webhook retries.
func (p *Plugin) handleCommentNotification(issueKey, author, comment string) {
	if p.store.IsNotificationThrottled("comment", issueKey) {
		p.API.LogDebug("webhook: comment notification throttled", "key", issueKey)
		return
	}

	issuePosts, err := p.store.GetIssuePosts(issueKey)
	if err != nil {
		p.API.LogError("webhook: store lookup failed for comment notification", "key", issueKey, "err", err.Error())
		return
	}
	if len(issuePosts) == 0 {
		return
	}

	t := p.translations()
	msg := fmt.Sprintf(t.CommentNotification, issueKey, escapeMarkdown(author))

	for _, ip := range issuePosts {
		post := &model.Post{
			UserId:    p.botUserID,
			ChannelId: ip.ChannelID,
			RootId:    ip.PostID, // reply in the thread of the issue card
			Message:   msg,
		}
		if _, appErr := p.API.CreatePost(post); appErr != nil {
			p.API.LogError("webhook: failed to post comment notification", "key", issueKey, "postID", ip.PostID, "err", appErr.Error())
		}
	}

	p.store.ThrottleNotification("comment", issueKey)
}

// handleAssignmentNotification announces a new assignee in each channel where the issue has a card.
// Throttled per issue to absorb Yandex Tracker webhook retries.
func (p *Plugin) handleAssignmentNotification(issueKey, assignee string) {
	if p.store.IsNotificationThrottled("assigned", issueKey) {
		p.API.LogDebug("webhook: assignment notification throttled", "key", issueKey)
		return
	}

	issuePosts, err := p.store.GetIssuePosts(issueKey)
	if err != nil {
		p.API.LogError("webhook: store lookup failed for assignment notification", "key", issueKey, "err", err.Error())
		return
	}
	if len(issuePosts) == 0 {
		return
	}

	t := p.translations()
	msg := fmt.Sprintf(t.AssignmentNotification, issueKey, escapeMarkdown(assignee))

	for _, ip := range issuePosts {
		post := &model.Post{
			UserId:    p.botUserID,
			ChannelId: ip.ChannelID,
			RootId:    ip.PostID, // reply in the thread of the issue card
			Message:   msg,
		}
		if _, appErr := p.API.CreatePost(post); appErr != nil {
			p.API.LogError("webhook: failed to post assignment notification", "key", issueKey, "postID", ip.PostID, "err", appErr.Error())
		}
	}

	p.store.ThrottleNotification("assigned", issueKey)
}

// validateWebhookSecret does a constant-time compare of X-Webhook-Secret.
// Fails closed: an unconfigured secret rejects all requests so the plugin never
// silently accepts unsigned webhook traffic.
func (p *Plugin) validateWebhookSecret(r *http.Request) bool {
	secret := p.getConfiguration().WebhookSecret
	if secret == "" {
		p.API.LogWarn("webhook: rejected — WebhookSecret is not configured; set it in the plugin settings and in Yandex Tracker")
		return false
	}
	provided := r.Header.Get("X-Webhook-Secret")
	return subtle.ConstantTimeCompare([]byte(provided), []byte(secret)) == 1
}
