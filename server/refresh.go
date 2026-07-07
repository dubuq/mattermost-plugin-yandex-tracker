package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dubuq/mattermost-plugin-yandex-tracker/server/tracker"
)

// startBackgroundRefresh starts a goroutine that periodically re-fetches all
// tracked issues and updates their cards. No-op if the interval is set to 0 (disabled).
// Safe to call from OnConfigurationChange — cancels any previous refresh loop first.
func (p *Plugin) startBackgroundRefresh() {
	hours := p.getConfiguration().refreshIntervalHours()
	if hours == 0 {
		p.API.LogInfo("Background card refresh disabled")
		return
	}

	p.refreshMu.Lock()
	if p.refreshCancel != nil {
		p.refreshCancel()
	}
	ctx, cancel := context.WithCancel(p.ctx)
	p.refreshCancel = cancel
	p.refreshMu.Unlock()

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		ticker := time.NewTicker(time.Duration(hours) * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				p.safeRunRefresh(ctx)
			case <-ctx.Done():
				return
			}
		}
	}()

	p.API.LogInfo("Background card refresh started", "intervalHours", hours)
}

// stopBackgroundRefresh cancels the current refresh loop if one is running.
func (p *Plugin) stopBackgroundRefresh() {
	p.refreshMu.Lock()
	defer p.refreshMu.Unlock()
	if p.refreshCancel != nil {
		p.refreshCancel()
		p.refreshCancel = nil
	}
}

// safeRunRefresh wraps runRefresh with a recover so a panic does not kill the goroutine.
func (p *Plugin) safeRunRefresh(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			func() {
				defer func() { recover() }()
				p.API.LogError("background refresh panicked, will retry on next tick", "err", fmt.Sprintf("%v", r))
			}()
		}
	}()
	p.runRefresh(ctx)
}

// trackedIssueEntry is used during refresh to sort issues by age for cap eviction.
type trackedIssueEntry struct {
	key         string
	lastUpdated time.Time
}

// runRefresh iterates all tracked issue keys, re-fetches any whose status has
// changed, and enqueues card updates. Logs a summary when done.
//
// Two lifecycle guards run each cycle:
//  1. Staleness: issues with no status change for cardStaleDays are expired from KV.
//  2. Cap: if total tracked issues exceed maxTrackedIssues, the oldest are evicted
//     until the count is back within the limit.
func (p *Plugin) runRefresh(ctx context.Context) {
	if !p.isConfigured() {
		return
	}

	p.API.LogDebug("Background refresh: starting")
	checked, updated, expired := 0, 0, 0

	staleThreshold := time.Duration(cardStaleDays) * 24 * time.Hour

	// Collect every active issue key and its last-updated time so we can enforce
	// the cap after the main processing loop without a second KV scan.
	var allTracked []trackedIssueEntry

	page := 0
	const perPage = 200
	for {
		keys, appErr := p.API.KVList(page, perPage)
		if appErr != nil {
			p.API.LogError("Background refresh: failed to list KV keys", "err", appErr.Error())
			return
		}

		for _, key := range keys {
			if !strings.HasPrefix(key, kvPrefixIssue) {
				continue
			}
			issueKey := key[len(kvPrefixIssue):]

			posts, err := p.store.GetIssuePosts(issueKey)
			if err != nil || len(posts) == 0 {
				continue
			}
			checked++

			client := p.getTrackerClient()
			if client == nil {
				// Configuration was cleared mid-cycle; stop this run cleanly.
				return
			}
			issue, err := client.GetIssue(tracker.ContextWithLocale(ctx, p.serverLocale()), issueKey)
			if err != nil {
				p.API.LogError("Background refresh: failed to fetch issue", "key", issueKey, "err", err.Error())
				continue
			}

			// Skip if status is unchanged — avoids spurious "(edited)" markers on posts.
			if issue.Status == p.store.GetLastStatus(issueKey) {
				lastUpdated := p.store.GetLastUpdated(issueKey)
				if lastUpdated.IsZero() {
					// No timestamp yet (existing installation). Plant one now so the
					// 7-day clock starts from this cycle rather than expiring immediately.
					p.store.SetLastUpdated(issueKey)
					lastUpdated = time.Now()
				} else if time.Since(lastUpdated) > staleThreshold {
					p.store.ExpireIssue(issueKey)
					p.API.LogInfo("Background refresh: expired idle issue", "key", issueKey,
						"idleDays", int(time.Since(lastUpdated).Hours()/24))
					expired++
					continue // removed — don't add to allTracked
				}
				allTracked = append(allTracked, trackedIssueEntry{key: issueKey, lastUpdated: lastUpdated})
				continue
			}

			p.enqueueUpdate(issueKey, issue)
			updated++
			allTracked = append(allTracked, trackedIssueEntry{key: issueKey, lastUpdated: p.store.GetLastUpdated(issueKey)})
		}

		if len(keys) < perPage {
			break
		}
		page++
	}

	// Cap enforcement: if we're over the limit, evict the oldest issues.
	if len(allTracked) > maxTrackedIssues {
		sort.Slice(allTracked, func(i, j int) bool {
			return allTracked[i].lastUpdated.Before(allTracked[j].lastUpdated)
		})
		toEvict := allTracked[:len(allTracked)-maxTrackedIssues]
		for _, e := range toEvict {
			p.store.ExpireIssue(e.key)
			expired++
		}
		p.API.LogWarn("Background refresh: cap exceeded, evicted oldest issues",
			"cap", maxTrackedIssues, "evicted", len(toEvict))
	}

	p.API.LogInfo("Background refresh complete", "checked", checked, "updated", updated, "expired", expired)

	// Prune orphaned KV entries after each cycle to keep the store bounded.
	if pruned := p.store.PruneOrphanedPosts(); pruned > 0 {
		p.API.LogInfo("Background refresh: pruned orphaned post entries", "pruned", pruned)
	}
}
