package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dubuq/mattermost-plugin-yandex-tracker/server/tracker"
	"github.com/dubuq/mattermost-plugin-yandex-tracker/server/tracker/yandex"
	"github.com/mattermost/mattermost/server/public/model"
)

const (
	yandexAuthorizeURL = "https://oauth.yandex.ru/authorize"
	yandexTokenURL     = "https://oauth.yandex.ru/token"
)

// oauthRedirectURI is the callback Yandex redirects to after user consent.
// Must be registered as the Redirect URI of the Yandex OAuth application.
func (p *Plugin) oauthRedirectURI() string {
	return fmt.Sprintf("%s/plugins/%s/oauth/complete", p.siteURL(), pluginID)
}

// tokenExpirySkewSecs refreshes tokens slightly before their actual expiry so
// an in-flight API call doesn't race the deadline.
const tokenExpirySkewSecs = 60

// getUserConnection returns the user's Tracker connection. Expired (or nearly
// expired) tokens are silently refreshed via the stored refresh token; the
// connection is only dropped when no refresh is possible, so the user is
// re-prompted to /tracker connect.
func (p *Plugin) getUserConnection(userID string) *UserConnection {
	conn := p.store.GetUserConnection(userID)
	if conn == nil {
		return nil
	}
	if conn.ExpiresAt == 0 || time.Now().Unix() < conn.ExpiresAt-tokenExpirySkewSecs {
		return conn
	}
	return p.refreshUserConnection(userID)
}

// refreshUserConnection exchanges the user's refresh token for a new access
// token and stores the updated connection. Returns nil (and removes the stored
// connection) when refresh is impossible or rejected — the caller then prompts
// the user to reconnect.
func (p *Plugin) refreshUserConnection(userID string) *UserConnection {
	// Serialized: a used Yandex refresh token is invalidated, so two
	// concurrent refreshes for the same user would kill the connection.
	p.oauthMu.Lock()
	defer p.oauthMu.Unlock()

	// Re-read under the lock — another goroutine may have just refreshed.
	conn := p.store.GetUserConnection(userID)
	if conn == nil {
		return nil
	}
	if conn.ExpiresAt == 0 || time.Now().Unix() < conn.ExpiresAt-tokenExpirySkewSecs {
		return conn
	}
	if conn.RefreshToken == "" {
		p.store.DeleteUserConnection(userID)
		return nil
	}

	cfg := p.getConfiguration()
	if !cfg.oauthConfigured() {
		return nil
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", conn.RefreshToken)
	tok, err := p.requestOAuthToken(form, cfg)
	if err != nil {
		p.API.LogWarn("oauth: token refresh failed — user must reconnect", "userID", userID, "err", err.Error())
		p.store.DeleteUserConnection(userID)
		return nil
	}

	conn.AccessToken = tok.AccessToken
	if tok.RefreshToken != "" {
		conn.RefreshToken = tok.RefreshToken
	}
	// ExpiresAt = 0 when Yandex omits expires_in: we then stop proactively
	// refreshing and fall back to the 401 → reconnect path, which is correct if
	// the token is later revoked. Yandex always returns expires_in in practice.
	conn.ExpiresAt = 0
	if tok.ExpiresIn > 0 {
		conn.ExpiresAt = time.Now().Unix() + tok.ExpiresIn
	}
	if err := p.store.SetUserConnection(userID, conn); err != nil {
		p.API.LogError("oauth: failed to store refreshed connection", "userID", userID, "err", err.Error())
		return nil
	}
	p.API.LogDebug("oauth: token refreshed", "userID", userID, "login", conn.Login)
	return conn
}

// getClientForUser returns a Tracker client authenticated as the given MM user,
// or nil if the user has not connected their account. All write operations
// (assign, transition, comment) must go through this client — never the
// service-account client — so actions are attributed to the real user.
func (p *Plugin) getClientForUser(userID string) (tracker.Client, *UserConnection) {
	conn := p.getUserConnection(userID)
	if conn == nil {
		return nil, nil
	}
	orgID := p.getConfiguration().OrgID
	if orgID == "" {
		// Connected user but no org configured: a server misconfiguration, not a
		// disconnected user. Log it so it isn't silently surfaced as a "connect"
		// prompt (isValid rejects an empty OrgID, so this should be unreachable).
		p.API.LogError("getClientForUser: OrgID not configured — cannot build per-user client", "userID", userID)
		return nil, nil
	}
	return yandex.New(conn.AccessToken, orgID), conn
}

// disconnectUser removes the user's stored connection. Called on explicit
// /tracker disconnect and when the API reports the token is no longer valid.
func (p *Plugin) disconnectUser(userID string) {
	p.store.DeleteUserConnection(userID)
}

// promptConnect sends an ephemeral message asking the user to connect their account.
func (p *Plugin) promptConnect(userID, channelID string) {
	p.sendUserEphemeral(userID, channelID, func(t Translations) string { return t.ConnectPrompt })
}

// promptReconnect tells the user their token was rejected and they must reconnect.
func (p *Plugin) promptReconnect(userID, channelID string) {
	p.disconnectUser(userID)
	p.sendUserEphemeral(userID, channelID, func(t Translations) string { return t.ReconnectPrompt })
}

// writeErrorMessage maps a per-user write failure to the text shown to the user.
// A rejected token (401) disconnects the user and returns the reconnect prompt;
// any other error returns the supplied fallback. Centralizes the 401→disconnect
// handling that the dialog- and JSON-based write handlers would otherwise repeat.
func (p *Plugin) writeErrorMessage(userID string, err error, fallback string) string {
	if errors.Is(err, tracker.ErrUnauthorized) {
		p.disconnectUser(userID)
		return p.translationsForUser(userID).ReconnectPrompt
	}
	return fallback
}

// sendUserEphemeral posts an ephemeral message in the user's own locale.
func (p *Plugin) sendUserEphemeral(userID, channelID string, msg func(Translations) string) {
	var locale string
	if user, appErr := p.API.GetUser(userID); appErr == nil {
		locale = user.Locale
	}
	p.API.SendEphemeralPost(userID, &model.Post{
		ChannelId: channelID,
		Message:   msg(translationsForLocale(locale)),
	})
}

// handleOAuthConnect starts the per-user OAuth flow: generates a CSRF state
// bound to the MM user and redirects the browser to the Yandex consent page.
func (p *Plugin) handleOAuthConnect(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-Id")
	if userID == "" {
		http.Error(w, "not authenticated", http.StatusUnauthorized)
		return
	}
	cfg := p.getConfiguration()
	if !cfg.oauthConfigured() {
		http.Error(w, "per-user authorization is not configured — ask your admin to set the Yandex OAuth Client ID and Secret", http.StatusNotImplemented)
		return
	}

	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	state := hex.EncodeToString(buf)
	if err := p.store.SetOAuthState(state, userID); err != nil {
		p.API.LogError("oauth: failed to store state", "err", err.Error())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", cfg.OAuthClientID)
	q.Set("redirect_uri", p.oauthRedirectURI())
	q.Set("state", state)
	// Always show the consent screen, even when Yandex remembers a previous
	// grant: the user may need to switch between Yandex accounts, and a
	// remembered grant keeps issuing tokens with the app's OLD scope set,
	// silently ignoring scopes added to the app later.
	q.Set("force_confirm", "yes")
	http.Redirect(w, r, yandexAuthorizeURL+"?"+q.Encode(), http.StatusFound)
}

// handleOAuthComplete is the redirect target of the Yandex consent page.
// Validates the CSRF state, exchanges the code for a token, resolves the
// user's Tracker login via /myself, and stores the connection.
func (p *Plugin) handleOAuthComplete(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-Id")
	if userID == "" {
		p.oauthResultPage(w, false, "You must be logged in to Mattermost in this browser.")
		return
	}

	query := r.URL.Query()
	if errCode := query.Get("error"); errCode != "" {
		p.oauthResultPage(w, false, "Yandex authorization was declined: "+errCode)
		return
	}

	code := query.Get("code")
	state := query.Get("state")
	if code == "" || state == "" {
		p.oauthResultPage(w, false, "Missing code or state parameter.")
		return
	}
	if stateUser := p.store.ConsumeOAuthState(state); stateUser == "" || stateUser != userID {
		p.oauthResultPage(w, false, "Invalid or expired authorization state. Please run /tracker connect again.")
		return
	}

	cfg := p.getConfiguration()
	tok, err := p.exchangeOAuthCode(code, cfg)
	if err != nil {
		p.API.LogError("oauth: token exchange failed", "err", err.Error())
		p.oauthResultPage(w, false, "Token exchange with Yandex failed. Please try again.")
		return
	}

	// Resolve the user's Tracker login with their own token — needed for "Assign to me".
	userClient := yandex.New(tok.AccessToken, cfg.OrgID)
	login, err := userClient.Myself(p.ctx)
	if err != nil {
		p.API.LogError("oauth: /myself failed after token exchange", "err", err.Error())
		p.oauthResultPage(w, false, "Connected to Yandex, but could not access Yandex Tracker with your account. Check that your OAuth app has Tracker permissions and that you have access to the organization.")
		return
	}

	conn := &UserConnection{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		Login:        login,
	}
	if tok.ExpiresIn > 0 {
		conn.ExpiresAt = time.Now().Unix() + tok.ExpiresIn
	}
	if err := p.store.SetUserConnection(userID, conn); err != nil {
		p.API.LogError("oauth: failed to store connection", "userID", userID, "err", err.Error())
		p.oauthResultPage(w, false, "Failed to store your connection. Please try again.")
		return
	}

	p.dmUser(userID, fmt.Sprintf("✅ Your Yandex Tracker account is connected as **%s**. Card actions (assign, status changes, comments) will now be performed on your behalf. Use `/tracker disconnect` to disconnect.", login))
	p.API.LogInfo("oauth: user connected", "userID", userID, "login", login)
	p.oauthResultPage(w, true, "Connected as "+login+". You can close this tab and return to Mattermost.")
}

type oauthToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

// exchangeOAuthCode swaps an authorization code for an access token at the Yandex OAuth endpoint.
func (p *Plugin) exchangeOAuthCode(code string, cfg *Configuration) (*oauthToken, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	return p.requestOAuthToken(form, cfg)
}

// requestOAuthToken posts a grant request (authorization_code or refresh_token)
// to the Yandex OAuth token endpoint. Client credentials are added here.
func (p *Plugin) requestOAuthToken(form url.Values, cfg *Configuration) (*oauthToken, error) {
	form.Set("client_id", cfg.OAuthClientID)
	form.Set("client_secret", cfg.OAuthClientSecret)

	ctx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, yandexTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var apiErr struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		if apiErr.Error != "" {
			return nil, fmt.Errorf("yandex oauth: %s (%s)", apiErr.Error, apiErr.ErrorDescription)
		}
		return nil, fmt.Errorf("yandex oauth: HTTP %d", resp.StatusCode)
	}

	var tok oauthToken
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if tok.AccessToken == "" {
		return nil, fmt.Errorf("yandex oauth: response has no access_token")
	}
	return &tok, nil
}

// dmUser sends a direct message from the bot to the user. Best-effort.
func (p *Plugin) dmUser(userID, message string) {
	channel, appErr := p.API.GetDirectChannel(p.botUserID, userID)
	if appErr != nil {
		p.API.LogWarn("dmUser: failed to get DM channel", "userID", userID, "err", appErr.Error())
		return
	}
	if _, appErr := p.API.CreatePost(&model.Post{
		UserId:    p.botUserID,
		ChannelId: channel.Id,
		Message:   message,
	}); appErr != nil {
		p.API.LogWarn("dmUser: failed to post", "userID", userID, "err", appErr.Error())
	}
}

// oauthResultTmpl renders the post-redirect page. html/template escapes every
// field contextually — critical because message carries values derived from
// Yandex-controlled query params (e.g. the OAuth error code), which would
// otherwise be a reflected-XSS sink on the Mattermost origin.
var oauthResultTmpl = template.Must(template.New("oauth-result").Parse(`<!DOCTYPE html>
<html><head><title>{{.Title}}</title></head>
<body style="font-family:sans-serif;display:flex;align-items:center;justify-content:center;height:100vh;margin:0">
<div style="text-align:center;max-width:480px">
<div style="font-size:48px">{{.Icon}}</div>
<h2>{{.Title}}</h2>
<p>{{.Message}}</p>
</div>
</body></html>`))

// oauthResultPage renders a minimal HTML page shown in the browser tab after the OAuth redirect.
func (p *Plugin) oauthResultPage(w http.ResponseWriter, ok bool, message string) {
	data := struct{ Title, Icon, Message string }{Title: "Yandex Tracker connected", Icon: "✅", Message: message}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if !ok {
		data.Title, data.Icon = "Connection failed", "❌"
		w.WriteHeader(http.StatusBadRequest)
	}
	if err := oauthResultTmpl.Execute(w, data); err != nil {
		p.API.LogError("oauth: failed to render result page", "err", err.Error())
	}
}
