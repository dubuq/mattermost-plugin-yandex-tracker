package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/dubuq/mattermost-plugin-yandex-tracker/server/tracker"
)

const (
	kvPrefixIssue         = "issue:"
	kvPrefixExpanded      = "expanded:"
	kvPrefixCache         = "fetchcache:"
	kvPrefixTransCache    = "transcache:"
	kvPrefixNotifCache    = "notifcache:"
	kvPrefixSubscription  = "subscription:"
	kvPrefixLastStatus    = "laststatus:"
	kvPrefixUserLogin     = "userlogin:"
	kvPrefixUserToken     = "usertoken:"    // userID → encrypted UserConnection JSON
	kvPrefixOAuthState    = "oauthstate:"   // state → userID (TTL oauthStateTTLSecs)
	kvKeyEncryptionKey    = "token_encryption_key"
	kvPrefixPostKeys      = "postkeys:"   // postID → []issueKey
	kvPrefixPostAttach    = "posta:"      // postID:issueKey → SlackAttachment JSON
	kvPrefixPending       = "pending:"      // userID → PendingTransition JSON
	kvPrefixLastUpdated   = "lastupdated:"  // issueKey → Unix timestamp of last card update
	cardStaleDays         = 7               // days of no status change before an issue is removed from tracking
	maxTrackedIssues      = 500             // hard cap on total tracked issues; oldest are evicted when exceeded
	expandedTTLSecs       = 24 * 60 * 60    // 24 hours
	fetchCacheTTLSecs     = 30               // seconds; prevents hammering the tracker API in busy channels
	transCacheTTLSecs     = 60               // seconds; warms the change-status dialog on first click so trigger ID doesn't expire
	notifCacheTTLSecs     = 10               // seconds; suppresses duplicate webhook retries from Yandex Tracker (retries arrive within 1-3s)
	pendingTTLSecs        = 5 * 60           // seconds; how long a pending transition is kept while the user fills fields
	oauthStateTTLSecs     = 10 * 60          // seconds; window for the user to complete the OAuth consent flow
	issuePostsCap         = 50               // max posts tracked per issue key
	subscriptionsCap      = 100              // max channels per queue subscription
)

// IssuePost records which MM post carries the preview card for an issue.
type IssuePost struct {
	ChannelID string `json:"channelID"`
	PostID    string `json:"postID"`
}

type Store struct {
	api plugin.API

	// encryptionKey encrypts per-user OAuth tokens at rest in the KV store.
	// Set once at activation via loadEncryptionKey; nil disables token storage.
	encryptionKey []byte
}

func NewStore(api plugin.API) *Store {
	return &Store{api: api}
}

// loadEncryptionKey loads the AES key used for user tokens, generating and
// persisting a new one on first activation.
//
// Threat model: the key lives in the same KV store as the ciphertext it
// protects, so an attacker with read access to the plugin KV store can decrypt
// tokens. Encryption here defends against exposure paths that don't include KV
// reads — e.g. raw DB backups filtered to the token rows, or logs — not against
// a full KV compromise. Separating the key (e.g. deriving it from a server-held
// secret) would harden this further.
func (s *Store) loadEncryptionKey() error {
	data, appErr := s.api.KVGet(kvKeyEncryptionKey)
	if appErr != nil {
		return fmt.Errorf("KVGet: %s", appErr.Error())
	}
	if len(data) == 32 {
		s.encryptionKey = data
		return nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return fmt.Errorf("generate key: %w", err)
	}
	if appErr := s.api.KVSet(kvKeyEncryptionKey, key); appErr != nil {
		return fmt.Errorf("KVSet: %s", appErr.Error())
	}
	s.encryptionKey = key
	return nil
}

func (s *Store) encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func (s *Store) decrypt(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(data) < gcm.NonceSize() {
		return nil, fmt.Errorf("ciphertext too short")
	}
	return gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
}

// UserConnection holds a user's personal Yandex Tracker OAuth credentials.
type UserConnection struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	Login        string `json:"login"`     // Tracker login, fetched via /myself at connect time
	ExpiresAt    int64  `json:"expiresAt"` // Unix seconds; 0 = unknown
}

// SetUserConnection stores a user's Tracker connection, encrypted at rest.
func (s *Store) SetUserConnection(userID string, conn *UserConnection) error {
	if s.encryptionKey == nil {
		return fmt.Errorf("encryption key not initialised")
	}
	data, err := json.Marshal(conn)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	enc, err := s.encrypt(data)
	if err != nil {
		return fmt.Errorf("encrypt: %w", err)
	}
	if appErr := s.api.KVSet(kvPrefixUserToken+userID, enc); appErr != nil {
		return fmt.Errorf("KVSet: %s", appErr.Error())
	}
	return nil
}

// GetUserConnection returns the user's Tracker connection, or nil if not connected
// (or the stored blob cannot be decrypted, e.g. after an encryption key reset).
func (s *Store) GetUserConnection(userID string) *UserConnection {
	if s.encryptionKey == nil {
		return nil
	}
	data, appErr := s.api.KVGet(kvPrefixUserToken + userID)
	if appErr != nil || data == nil {
		return nil
	}
	plain, err := s.decrypt(data)
	if err != nil {
		return nil
	}
	var conn UserConnection
	if err := json.Unmarshal(plain, &conn); err != nil {
		return nil
	}
	return &conn
}

// DeleteUserConnection removes the user's Tracker connection.
func (s *Store) DeleteUserConnection(userID string) {
	_ = s.api.KVDelete(kvPrefixUserToken + userID)
}

// SetOAuthState stores a CSRF state token for the OAuth flow. Expires after oauthStateTTLSecs.
func (s *Store) SetOAuthState(state, userID string) error {
	if appErr := s.api.KVSetWithExpiry(kvPrefixOAuthState+state, []byte(userID), oauthStateTTLSecs); appErr != nil {
		return fmt.Errorf("KVSetWithExpiry: %s", appErr.Error())
	}
	return nil
}

// ConsumeOAuthState returns the userID bound to state and deletes it (one-time use).
// Returns empty string if the state is unknown or expired.
func (s *Store) ConsumeOAuthState(state string) string {
	data, appErr := s.api.KVGet(kvPrefixOAuthState + state)
	if appErr != nil || data == nil {
		return ""
	}
	_ = s.api.KVDelete(kvPrefixOAuthState + state)
	return string(data)
}

// SaveIssuePost records that postID carries a card for issueKey. Capped at issuePostsCap; oldest dropped when full.
func (s *Store) SaveIssuePost(issueKey, channelID, postID string) error {
	posts, err := s.GetIssuePosts(issueKey)
	if err != nil {
		return err
	}
	for _, p := range posts {
		if p.PostID == postID {
			return nil // already tracked
		}
	}
	posts = append(posts, IssuePost{ChannelID: channelID, PostID: postID})
	if len(posts) > issuePostsCap {
		posts = posts[len(posts)-issuePostsCap:]
	}
	data, err := json.Marshal(posts)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if appErr := s.api.KVSet(kvPrefixIssue+issueKey, data); appErr != nil {
		return fmt.Errorf("KVSet: %s", appErr.Error())
	}
	return nil
}

// GetIssuePosts returns all posts carrying a card for issueKey.
func (s *Store) GetIssuePosts(issueKey string) ([]IssuePost, error) {
	data, appErr := s.api.KVGet(kvPrefixIssue + issueKey)
	if appErr != nil {
		return nil, fmt.Errorf("KVGet: %s", appErr.Error())
	}
	if data == nil {
		return nil, nil
	}
	// Handle legacy single-object format written by older plugin versions.
	if len(data) > 0 && data[0] == '{' {
		var single IssuePost
		if err := json.Unmarshal(data, &single); err != nil {
			return nil, fmt.Errorf("unmarshal legacy: %w", err)
		}
		return []IssuePost{single}, nil
	}
	var posts []IssuePost
	if err := json.Unmarshal(data, &posts); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return posts, nil
}

// MarkExpanded records that issueKey was expanded on postID. Expires after 24h so edits can re-expand.
func (s *Store) MarkExpanded(postID, issueKey string) error {
	key := expandedKey(postID, issueKey)
	if appErr := s.api.KVSetWithExpiry(key, []byte("1"), expandedTTLSecs); appErr != nil {
		return fmt.Errorf("KVSetWithExpiry: %s", appErr.Error())
	}
	return nil
}

func (s *Store) IsExpanded(postID, issueKey string) bool {
	data, appErr := s.api.KVGet(expandedKey(postID, issueKey))
	return appErr == nil && data != nil
}

func expandedKey(postID, issueKey string) string {
	return fmt.Sprintf("%s%s:%s", kvPrefixExpanded, postID, issueKey)
}

// GetCachedIssue returns a recently fetched issue, or nil if missing/expired.
func (s *Store) GetCachedIssue(issueKey string) *tracker.Issue {
	data, appErr := s.api.KVGet(kvPrefixCache + issueKey)
	if appErr != nil || data == nil {
		return nil
	}
	var issue tracker.Issue
	if err := json.Unmarshal(data, &issue); err != nil {
		return nil
	}
	return &issue
}

// CacheIssue stores an issue for fetchCacheTTLSecs seconds.
// Errors are silently ignored — the cache is best-effort.
func (s *Store) CacheIssue(issueKey string, issue *tracker.Issue) {
	data, err := json.Marshal(issue)
	if err != nil {
		return
	}
	_ = s.api.KVSetWithExpiry(kvPrefixCache+issueKey, data, fetchCacheTTLSecs)
}

// GetCachedTransitions returns recently fetched transitions for an issue, or nil if missing/expired.
func (s *Store) GetCachedTransitions(issueKey string) []tracker.Transition {
	data, appErr := s.api.KVGet(kvPrefixTransCache + issueKey)
	if appErr != nil || len(data) == 0 {
		return nil
	}
	var transitions []tracker.Transition
	if err := json.Unmarshal(data, &transitions); err != nil {
		return nil
	}
	return transitions
}

// CacheTransitions stores transitions for transCacheTTLSecs seconds.
func (s *Store) CacheTransitions(issueKey string, transitions []tracker.Transition) {
	data, err := json.Marshal(transitions)
	if err != nil {
		return
	}
	_ = s.api.KVSetWithExpiry(kvPrefixTransCache+issueKey, data, transCacheTTLSecs)
}

// InvalidateCachedTransitions removes the transitions cache for an issue.
// Call after a successful ExecuteTransition so the next change-status click
// fetches fresh transitions for the new state instead of serving stale IDs.
func (s *Store) InvalidateCachedTransitions(issueKey string) {
	_ = s.api.KVDelete(kvPrefixTransCache + issueKey)
}

// IsNotificationThrottled returns true if a notification was recently sent for this event+key.
func (s *Store) IsNotificationThrottled(eventType, issueKey string) bool {
	data, appErr := s.api.KVGet(notifCacheKey(eventType, issueKey))
	return appErr == nil && data != nil
}

// ThrottleNotification marks an event+key as recently notified (expires after notifCacheTTLSecs).
func (s *Store) ThrottleNotification(eventType, issueKey string) {
	_ = s.api.KVSetWithExpiry(notifCacheKey(eventType, issueKey), []byte("1"), notifCacheTTLSecs)
}

func notifCacheKey(eventType, issueKey string) string {
	return fmt.Sprintf("%s%s:%s", kvPrefixNotifCache, eventType, issueKey)
}

// GetSubscribedChannels returns the channel IDs subscribed to queueKey.
// Schema: "subscription:<QUEUE>" → []string{channelID, ...}
func (s *Store) GetSubscribedChannels(queueKey string) ([]string, error) {
	data, appErr := s.api.KVGet(kvPrefixSubscription + queueKey)
	if appErr != nil {
		return nil, fmt.Errorf("KVGet: %s", appErr.Error())
	}
	if data == nil {
		return nil, nil
	}
	var channels []string
	if err := json.Unmarshal(data, &channels); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return channels, nil
}

// AddSubscription subscribes channelID to queueKey. Idempotent.
func (s *Store) AddSubscription(channelID, queueKey string) error {
	channels, err := s.GetSubscribedChannels(queueKey)
	if err != nil {
		return err
	}
	for _, c := range channels {
		if c == channelID {
			return nil // already subscribed
		}
	}
	channels = append(channels, channelID)
	if len(channels) > subscriptionsCap {
		channels = channels[len(channels)-subscriptionsCap:]
	}
	data, err := json.Marshal(channels)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if appErr := s.api.KVSet(kvPrefixSubscription+queueKey, data); appErr != nil {
		return fmt.Errorf("KVSet: %s", appErr.Error())
	}
	return nil
}

// RemoveSubscription unsubscribes channelID from queueKey. No-op if not subscribed.
func (s *Store) RemoveSubscription(channelID, queueKey string) error {
	channels, err := s.GetSubscribedChannels(queueKey)
	if err != nil {
		return err
	}
	filtered := channels[:0]
	for _, c := range channels {
		if c != channelID {
			filtered = append(filtered, c)
		}
	}
	if len(filtered) == 0 {
		if appErr := s.api.KVDelete(kvPrefixSubscription + queueKey); appErr != nil {
			return fmt.Errorf("KVDelete: %s", appErr.Error())
		}
		return nil
	}
	data, err := json.Marshal(filtered)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if appErr := s.api.KVSet(kvPrefixSubscription+queueKey, data); appErr != nil {
		return fmt.Errorf("KVSet: %s", appErr.Error())
	}
	return nil
}

// GetChannelQueues returns all queue keys that channelID is subscribed to.
// Requires a full KV scan — only call from user-triggered slash commands, not hot paths.
func (s *Store) GetChannelQueues(channelID string) ([]string, error) {
	var queues []string
	page := 0
	const perPage = 200
	for {
		keys, appErr := s.api.KVList(page, perPage)
		if appErr != nil {
			return nil, fmt.Errorf("KVList: %s", appErr.Error())
		}
		for _, key := range keys {
			if !strings.HasPrefix(key, kvPrefixSubscription) {
				continue
			}
			queueKey := key[len(kvPrefixSubscription):]
			channels, err := s.GetSubscribedChannels(queueKey)
			if err != nil {
				continue
			}
			for _, c := range channels {
				if c == channelID {
					queues = append(queues, queueKey)
					break
				}
			}
		}
		if len(keys) < perPage {
			break
		}
		page++
	}
	return queues, nil
}

// GetLastStatus returns the last recorded status for an issue key, or empty string.
func (s *Store) GetLastStatus(issueKey string) string {
	data, appErr := s.api.KVGet(kvPrefixLastStatus + issueKey)
	if appErr != nil || data == nil {
		return ""
	}
	return string(data)
}

// SetLastStatus records the last known status for an issue key and bumps the
// last-updated timestamp so the card lifecycle timer resets on each real update.
// Errors are silently ignored — this is best-effort metadata for the background refresh.
func (s *Store) SetLastStatus(issueKey, status string) {
	_ = s.api.KVSet(kvPrefixLastStatus+issueKey, []byte(status))
	s.SetLastUpdated(issueKey)
}

// SetLastUpdated records the current time as the last activity timestamp for issueKey.
// Called by SetLastStatus so it advances whenever cards are successfully updated.
func (s *Store) SetLastUpdated(issueKey string) {
	ts := fmt.Sprintf("%d", time.Now().Unix())
	_ = s.api.KVSet(kvPrefixLastUpdated+issueKey, []byte(ts))
}

// GetLastUpdated returns the last time cards for issueKey were successfully updated.
// Returns the zero time if no timestamp has been stored yet (e.g. freshly tracked issues
// on an existing installation that predates this feature).
func (s *Store) GetLastUpdated(issueKey string) time.Time {
	data, appErr := s.api.KVGet(kvPrefixLastUpdated + issueKey)
	if appErr != nil || data == nil {
		return time.Time{}
	}
	var ts int64
	if _, err := fmt.Sscan(string(data), &ts); err != nil {
		return time.Time{}
	}
	return time.Unix(ts, 0)
}

// ExpireIssue removes all card-lifecycle KV entries for issueKey:
// the post list, last-status, last-updated, and all per-post attachment caches.
// Subscription data and user logins are intentionally preserved.
func (s *Store) ExpireIssue(issueKey string) {
	posts, _ := s.GetIssuePosts(issueKey)
	_ = s.api.KVDelete(kvPrefixIssue + issueKey)
	_ = s.api.KVDelete(kvPrefixLastStatus + issueKey)
	_ = s.api.KVDelete(kvPrefixLastUpdated + issueKey)
	for _, ip := range posts {
		_ = s.api.KVDelete(kvPrefixPostAttach + ip.PostID + ":" + issueKey)
		s.RemovePostIssueKey(ip.PostID, issueKey)
	}
}

// AddPostIssueKey records that postID contains a card for issueKey. Idempotent.
func (s *Store) AddPostIssueKey(postID, issueKey string) {
	keys := s.GetPostIssueKeys(postID)
	for _, k := range keys {
		if k == issueKey {
			return
		}
	}
	keys = append(keys, issueKey)
	data, err := json.Marshal(keys)
	if err != nil {
		return
	}
	_ = s.api.KVSet(kvPrefixPostKeys+postID, data)
}

// GetPostIssueKeys returns all issue keys that have cards in postID.
func (s *Store) GetPostIssueKeys(postID string) []string {
	data, appErr := s.api.KVGet(kvPrefixPostKeys + postID)
	if appErr != nil || data == nil {
		return nil
	}
	var keys []string
	if err := json.Unmarshal(data, &keys); err != nil {
		return nil
	}
	return keys
}

// RemoveIssuePost removes postID from the list of posts tracked for issueKey.
// Called when UpdatePost fails with a deleted-channel error so future webhook
// updates no longer attempt to write to a post that can never be updated.
func (s *Store) RemoveIssuePost(issueKey, postID string) {
	posts, err := s.GetIssuePosts(issueKey)
	if err != nil || len(posts) == 0 {
		return
	}
	filtered := posts[:0]
	for _, p := range posts {
		if p.PostID != postID {
			filtered = append(filtered, p)
		}
	}
	if len(filtered) == 0 {
		_ = s.api.KVDelete(kvPrefixIssue + issueKey)
		return
	}
	data, _ := json.Marshal(filtered)
	_ = s.api.KVSet(kvPrefixIssue+issueKey, data)
}

func (s *Store) RemovePostIssueKey(postID, issueKey string) {
	keys := s.GetPostIssueKeys(postID)
	filtered := keys[:0]
	for _, k := range keys {
		if k != issueKey {
			filtered = append(filtered, k)
		}
	}
	if len(filtered) == 0 {
		_ = s.api.KVDelete(kvPrefixPostKeys + postID)
		return
	}
	data, _ := json.Marshal(filtered)
	_ = s.api.KVSet(kvPrefixPostKeys+postID, data)
}

// SetPostAttachment stores the rendered attachment for (postID, issueKey).
// Source of truth for card rebuilds — never read from post.Props which can be corrupted.
func (s *Store) SetPostAttachment(postID, issueKey string, a *model.SlackAttachment) {
	if a == nil {
		return
	}
	// Guard against corrupted string fields that cause json.Marshal to panic.
	if a.Title == "" && a.Text == "" && a.Fallback == "" {
		return
	}
	var data []byte
	var err error
	func() {
		defer func() { recover() }()
		data, err = json.Marshal(a)
	}()
	if err != nil || data == nil {
		return
	}
	_ = s.api.KVSet(kvPrefixPostAttach+postID+":"+issueKey, data)
}

func (s *Store) GetPostAttachment(postID, issueKey string) *model.SlackAttachment {
	data, appErr := s.api.KVGet(kvPrefixPostAttach + postID + ":" + issueKey)
	if appErr != nil || data == nil {
		return nil
	}
	var a model.SlackAttachment
	if err := json.Unmarshal(data, &a); err != nil {
		return nil
	}
	return &a
}

// ClearPostCache removes all post-related KV entries, preserving subscriptions and logins.
func (s *Store) ClearPostCache() (int, error) {
	page := 0
	const perPage = 200
	deleted := 0
	for {
		keys, appErr := s.api.KVList(page, perPage)
		if appErr != nil {
			return deleted, fmt.Errorf("KVList: %s", appErr.Error())
		}
		for _, key := range keys {
			if strings.HasPrefix(key, kvPrefixSubscription) || strings.HasPrefix(key, kvPrefixUserLogin) ||
				strings.HasPrefix(key, kvPrefixUserToken) || key == kvKeyEncryptionKey {
				continue
			}
			if delErr := s.api.KVDelete(key); delErr == nil {
				deleted++
			}
		}
		if len(keys) < perPage {
			break
		}
		page++
	}
	return deleted, nil
}

// PendingTransition holds a transition that needs additional fields before it can execute.
// Stored keyed by the MM user ID so concurrent users don't collide.
type PendingTransition struct {
	IssueKey       string                       `json:"issueKey"`
	PostID         string                       `json:"postID"`
	TransitionID   string                       `json:"transitionID"`
	TransitionName string                       `json:"transitionName"`
	// Fields maps each field name to its label→apiKey pairs.
	// Non-empty map → select dropdown (label shown to user, key sent to Tracker).
	// Empty map {} → free-text input (value sent as plain string).
	Fields         map[string]map[string]string `json:"fields"`
}

// SetPendingTransition stores a pending transition for userID. Expires after pendingTTLSecs.
func (s *Store) SetPendingTransition(userID string, pt PendingTransition) error {
	data, err := json.Marshal(pt)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if appErr := s.api.KVSetWithExpiry(kvPrefixPending+userID, data, pendingTTLSecs); appErr != nil {
		return fmt.Errorf("KVSetWithExpiry: %s", appErr.Error())
	}
	return nil
}

// GetPendingTransition retrieves the pending transition for userID, or nil if none exists / expired.
func (s *Store) GetPendingTransition(userID string) (*PendingTransition, error) {
	data, appErr := s.api.KVGet(kvPrefixPending + userID)
	if appErr != nil {
		return nil, fmt.Errorf("KVGet: %s", appErr.Error())
	}
	if data == nil {
		return nil, nil
	}
	var pt PendingTransition
	if err := json.Unmarshal(data, &pt); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return &pt, nil
}

// DeletePendingTransition removes any pending transition for userID.
func (s *Store) DeletePendingTransition(userID string) {
	_ = s.api.KVDelete(kvPrefixPending + userID)
}

// PruneOrphanedPosts removes KV data for posts that no longer exist. Returns count pruned.
func (s *Store) PruneOrphanedPosts() int {
	page := 0
	const perPage = 200
	pruned := 0
	for {
		keys, appErr := s.api.KVList(page, perPage)
		if appErr != nil {
			break
		}
		for _, key := range keys {
			if !strings.HasPrefix(key, kvPrefixPostKeys) {
				continue
			}
			postID := key[len(kvPrefixPostKeys):]
			if _, err := s.api.GetPost(postID); err == nil {
				continue // post still exists, keep it
			}
			// Post gone — remove postkeys entry and all posta: entries for it.
			issueKeys := s.GetPostIssueKeys(postID)
			_ = s.api.KVDelete(kvPrefixPostKeys + postID)
			for _, ik := range issueKeys {
				_ = s.api.KVDelete(kvPrefixPostAttach + postID + ":" + ik)
			}
			pruned++
		}
		if len(keys) < perPage {
			break
		}
		page++
	}
	return pruned
}
