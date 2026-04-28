# Changelog

## [0.1.0] — 2026-04-26

First public release.

### Added

**Inline issue preview cards**
- Automatically detects Yandex Tracker issue keys (e.g. `DEV-123`) and full tracker URLs in messages and replaces them with rich preview cards
- Cards show issue title, status, priority, type, and assignee
- Collapsed by default; expand to see all fields
- Collapse / Expand / Refresh / Dismiss action buttons on each card
- Multiple issue keys in a single message each get their own card
- Posting the same issue key multiple times in a channel does not create duplicate cards

**Webhook-driven live updates**
- Cards update in-place across all channels when a Yandex Tracker webhook fires
- Supported event types: status change, comment added, assignee changed, issue created
- Comment and assignment events post a notification reply in the thread of the issue card
- Optional `X-Webhook-Secret` header validation to reject unauthorized requests
- Deduplication of rapid webhook retries (10-second throttle per event)

**Write actions (expanded card only)**
- **Assign to me** — assigns the issue to the MM user's Yandex Tracker login; login is saved after first use
- **Change Status** — opens a dialog listing available transitions for the current issue state
- **Required fields flow** — if a transition requires additional fields (e.g. resolution), an intermediate dialog collects them before executing the transition; fields are configured per-transition by the admin

**Queue subscriptions**
- `/tracker subscribe QUEUE` — new issues created in QUEUE are automatically posted as cards in the current channel
- `/tracker unsubscribe QUEUE` — removes the subscription
- `/tracker subscriptions` — lists all queues the current channel is subscribed to

**Slash command**
- `/tracker KEY` — posts an ephemeral preview card for any issue key

**Background card refresh**
- Periodically re-fetches all tracked issues and updates cards when status has changed, catching any webhooks that were missed
- Configurable interval (1–24 hours) or disabled
- Card lifecycle: issues with no status change for 7 days are automatically removed from tracking; cards can be re-created by pasting the issue key again
- Hard cap of 500 simultaneously tracked issues; oldest are evicted if the limit is exceeded

**Configuration (System Console)**
- Tracker OAuth token and Organization ID
- Webhook secret
- Bot display name
- Monitor All Channels toggle — when disabled, the bot only monitors channels it has been added to as a member
- Background refresh interval
- Configurable status colors (Active / Done / Cancelled / Default / Custom) with comma-separated status name lists
- Required fields per transition (JSON config)

**Internationalization**
- UI strings available in 10 languages: English, Russian, German, French, Spanish, Polish, Ukrainian, Kazakh, Uzbek, Turkish
- Language follows the Mattermost server's default client locale setting

**Admin utilities**
- **Test Connection** button — verifies the configured token and org ID against the Tracker API
- **Clear Cache** button — wipes all post-mapping KV data; does not affect subscriptions or user logins

### Notes

- Requires Mattermost Server 9.x or later
- Bot must be added to each team before it can be invited to individual channels (when Monitor All Channels is disabled)
- Yandex Tracker webhooks must be configured manually; see plugin README for trigger templates
