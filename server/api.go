package main

import (
	"net/http"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

// maxRequestBodyBytes caps the size of any request body the plugin will decode.
// Interactive action payloads and webhook events are small; this bounds memory
// use and protects the JSON decoders against oversized/hostile bodies.
const maxRequestBodyBytes = 1 << 20 // 1 MiB

// authedHandler is an HTTP handler that has already resolved the authenticated
// Mattermost user ID from the request.
type authedHandler func(w http.ResponseWriter, r *http.Request, userID string)

// ServeHTTP routes plugin HTTP requests. MM forwards /plugins/{plugin-id}/{path} here with r.URL.Path = /{path}.
//
// Endpoints fall into three trust tiers:
//   - webhook: authenticated by the shared X-Webhook-Secret (no MM session).
//   - user endpoints: require a valid MM session (requireUser).
//   - admin endpoints: require a MM session with system-admin rights (requireAdmin).
//
// The OAuth endpoints resolve the session themselves so they can render a
// friendly HTML/redirect response instead of a bare 401.
func (p *Plugin) ServeHTTP(_ *plugin.Context, w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/webhook":
		p.safeServe("webhook", p.limitBody(p.handleWebhook))(w, r)
	case "/refresh":
		p.safeServe("refresh", p.requireUser(p.handleRefresh))(w, r)
	case "/dismiss":
		p.safeServe("dismiss", p.requireUser(p.handleDismiss))(w, r)
	case "/collapse":
		p.safeServe("collapse", p.requireUser(p.handleCollapse))(w, r)
	case "/expand":
		p.safeServe("expand", p.requireUser(p.handleExpand))(w, r)
	case "/assign":
		p.safeServe("assign", p.requireUser(p.handleAssignToMe))(w, r)
	case "/oauth/connect":
		p.safeServe("oauth-connect", p.limitBody(p.handleOAuthConnect))(w, r)
	case "/oauth/complete":
		p.safeServe("oauth-complete", p.limitBody(p.handleOAuthComplete))(w, r)
	case "/change-status":
		p.safeServe("change-status", p.requireUser(p.handleChangeStatus))(w, r)
	case "/transition":
		p.safeServe("transition", p.requireUser(p.handleTransition))(w, r)
	case "/webhook-url":
		p.safeServe("webhook-url", p.requireAdmin(p.handleWebhookURL))(w, r)
	case "/verify":
		p.safeServe("verify", p.requireAdmin(p.handleVerify))(w, r)
	case "/clear-cache":
		p.safeServe("clear-cache", p.requireAdmin(p.handleClearCache))(w, r)
	case "/add-comment":
		p.safeServe("add-comment", p.requireUser(p.handleAddComment))(w, r)
	case "/fill-fields":
		p.safeServe("fill-fields", p.requireUser(p.handleFillFields))(w, r)
	case "/fill-fields-submit":
		p.safeServe("fill-fields-submit", p.requireUser(p.handleFillFieldsSubmit))(w, r)
	default:
		http.NotFound(w, r)
	}
}

// limitBody caps the request body size before the handler decodes it.
func (p *Plugin) limitBody(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
		fn(w, r)
	}
}

// requireUser enforces that the request carries a valid Mattermost session and
// passes the resolved user ID to the handler. The Mattermost-User-Id header is
// set by the server only for authenticated requests; any client-supplied value
// is stripped, so it is the sole trustworthy source of the caller's identity.
// Never authorize based on a user ID taken from the request body.
func (p *Plugin) requireUser(fn authedHandler) http.HandlerFunc {
	return p.limitBody(func(w http.ResponseWriter, r *http.Request) {
		userID := r.Header.Get("Mattermost-User-Id")
		if userID == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !p.actionLimiter.allow(userID) {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		fn(w, r, userID)
	})
}

// requireAdmin enforces an authenticated session with system-administrator
// rights. Used for the System Console-backed endpoints.
func (p *Plugin) requireAdmin(fn authedHandler) http.HandlerFunc {
	return p.requireUser(func(w http.ResponseWriter, r *http.Request, userID string) {
		if !p.API.HasPermissionTo(userID, model.PermissionManageSystem) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		fn(w, r, userID)
	})
}

// bodyUserMatches reports whether a user ID taken from a request body is
// consistent with the authenticated session user. Mattermost populates the body
// user ID server-side for genuine interactive requests, so an empty value is
// tolerated but a non-empty mismatch indicates a forged payload and is rejected.
func bodyUserMatches(bodyUserID, authUserID string) bool {
	return bodyUserID == "" || bodyUserID == authUserID
}

// canReadPost reports whether userID may read the given post — i.e. has read
// access to the post's channel. Used to gate every handler that accepts a
// client-supplied post ID so a caller cannot read or mutate posts in channels
// they are not a member of.
func (p *Plugin) canReadPost(userID string, post *model.Post) bool {
	return p.API.HasPermissionToChannel(userID, post.ChannelId, model.PermissionReadChannel)
}
