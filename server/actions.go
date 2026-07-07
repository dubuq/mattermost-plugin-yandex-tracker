package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/dubuq/mattermost-plugin-yandex-tracker/server/tracker"
)

// handleDismiss removes the card for an issue from the post.
func (p *Plugin) handleDismiss(w http.ResponseWriter, r *http.Request, userID string) {
	var req model.PostActionIntegrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if !bodyUserMatches(req.UserId, userID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	issueKey, _ := req.Context["issue_key"].(string)

	post, appErr := p.API.GetPost(req.PostId)
	if appErr != nil {
		p.API.LogError("dismiss: failed to get post", "postID", req.PostId, "err", appErr.Error())
		writeActionResponse(w, nil)
		return
	}
	if !p.canReadPost(userID, post) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// Rebuild attachment list from KV, excluding the dismissed key.
	// Never read from post.Props (may contain corrupted pointer data).
	allKeys := p.store.GetPostIssueKeys(req.PostId)
	filtered := make([]*model.SlackAttachment, 0, len(allKeys))
	for _, k := range allKeys {
		if k == issueKey {
			continue
		}
		if a := p.store.GetPostAttachment(req.PostId, k); a != nil {
			filtered = append(filtered, a)
		}
	}

	ensureProps(post)
	if len(filtered) == 0 {
		delete(post.Props, "attachments")
	} else {
		post.Props["attachments"] = filtered
	}

	p.store.RemovePostIssueKey(req.PostId, issueKey)

	// Return the desired state in Update — MM performs the UpdatePost and refreshes
	// action IDs. Calling p.API.UpdatePost here too would cause "Invalid action id" errors.
	writeActionResponse(w, &model.PostActionIntegrationResponse{Update: post})
}

// handleRefresh enqueues a card update. Responds immediately — GetIssue can take
// seconds and a blocking handler risks an MM muxBroker timeout killing the plugin.
//
// postID and channelID are included in the job so the worker re-registers the
// post mapping if it was lost (e.g. after a zombie episode or plugin restart).
// This makes the refresh button self-healing for "dead" cards.
func (p *Plugin) handleRefresh(w http.ResponseWriter, r *http.Request, userID string) {
	var req model.PostActionIntegrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if !bodyUserMatches(req.UserId, userID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	issueKey, _ := req.Context["issue_key"].(string)
	if !validIssueKey(issueKey) || !p.isConfigured() {
		writeActionResponse(w, nil)
		return
	}

	// The refresh job re-registers the post mapping and updates the card via the
	// worker, which will GetPost/UpdatePost this post ID. Verify the caller can
	// actually read the post's channel here — otherwise a forged post_id could
	// stamp a card onto (and overwrite the attachments of) a post in a channel
	// the user cannot access. Silent no-op keeps behaviour consistent with the
	// other card handlers and doesn't reveal whether the post exists.
	post, appErr := p.API.GetPost(req.PostId)
	if appErr != nil || !p.canReadPost(userID, post) {
		writeActionResponse(w, nil)
		return
	}

	// Per-user throttle: coalesce repeated manual refreshes of the same issue so a
	// user cannot drive unbounded forced fetches against the Tracker API.
	if p.store.IsNotificationThrottled("refresh", issueKey) {
		writeActionResponse(w, nil)
		return
	}
	p.store.ThrottleNotification("refresh", issueKey)

	p.enqueueJob(updateJob{
		issueKey:     issueKey,
		forceRefresh: true,
		postID:       req.PostId,
		channelID:    req.ChannelId,
	})
	writeActionResponse(w, nil)
}

func (p *Plugin) handleCollapse(w http.ResponseWriter, r *http.Request, userID string) {
	p.toggleCollapse(w, r, userID, true)
}

func (p *Plugin) handleExpand(w http.ResponseWriter, r *http.Request, userID string) {
	p.toggleCollapse(w, r, userID, false)
}

func (p *Plugin) toggleCollapse(w http.ResponseWriter, r *http.Request, userID string, collapse bool) {
	var req model.PostActionIntegrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if !bodyUserMatches(req.UserId, userID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	issueKey, _ := req.Context["issue_key"].(string)
	if !validIssueKey(issueKey) || !p.isConfigured() {
		writeActionResponse(w, nil)
		return
	}

	action := "expand"
	if collapse {
		action = "collapse"
	}

	post, appErr := p.API.GetPost(req.PostId)
	if appErr != nil {
		p.API.LogError(action+": failed to get post", "postID", req.PostId, "key", issueKey, "err", appErr.Error())
		writeActionResponse(w, nil)
		return
	}
	if !p.canReadPost(userID, post) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	client := p.getTrackerClient()
	if client == nil {
		writeActionResponse(w, nil)
		return
	}

	// Use cache to avoid an extra API call just for a UI toggle.
	issue := p.store.GetCachedIssue(issueKey)
	if issue == nil {
		var err error
		issue, err = client.GetIssue(tracker.ContextWithLocale(p.ctx, p.serverLocale()), issueKey)
		if err != nil {
			p.API.LogError(action+": failed to fetch issue", "key", issueKey, "err", err.Error())
			writeActionResponse(w, nil)
			return
		}
		p.store.CacheIssue(issueKey, issue)
	}

	cfg := p.getConfiguration()
	attachment := p.formatter.BuildAttachment(issue, cfg.resolveStatusColor(issue.Status), p.translations(), collapse, true)

	// Rebuild full attachment list from KV — never read from post.Props.
	allKeys := p.store.GetPostIssueKeys(req.PostId)
	if len(allKeys) == 0 {
		allKeys = []string{issueKey}
	}
	allAttachments := make([]*model.SlackAttachment, 0, len(allKeys))
	for _, k := range allKeys {
		if k == issueKey {
			allAttachments = append(allAttachments, attachment)
		} else if a := p.store.GetPostAttachment(req.PostId, k); a != nil {
			allAttachments = append(allAttachments, a)
		}
	}
	p.store.SetPostAttachment(req.PostId, issueKey, attachment)

	ensureProps(post)
	post.Props["attachments"] = allAttachments

	writeActionResponse(w, &model.PostActionIntegrationResponse{Update: post})
}

// handleAssignToMe assigns the issue to the clicking user's connected Tracker
// account. The user's own OAuth token is used, so Tracker attributes the change
// to them — never to the service account. Non-connected users get an ephemeral
// prompt to run /tracker connect.
func (p *Plugin) handleAssignToMe(w http.ResponseWriter, r *http.Request, userID string) {
	var req model.PostActionIntegrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if !bodyUserMatches(req.UserId, userID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	issueKey, _ := req.Context["issue_key"].(string)
	if !validIssueKey(issueKey) {
		writeActionResponse(w, nil)
		return
	}

	client, conn := p.getClientForUser(userID)
	if client == nil {
		p.promptConnect(userID, req.ChannelId)
		writeActionResponse(w, nil)
		return
	}

	// Copy the fields needed by the goroutine — do not capture req directly.
	channelID := req.ChannelId
	login := conn.Login

	// Respond immediately — AssignIssue is a blocking HTTP call and a slow
	// handler risks an MM muxBroker timeout killing the plugin.
	writeActionResponse(w, nil)

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		if p.ctx.Err() != nil {
			return
		}

		if err := client.AssignIssue(p.ctx, issueKey, login); err != nil {
			p.API.LogError("assign: failed to assign issue", "key", issueKey, "userID", userID, "err", err.Error())
			if errors.Is(err, tracker.ErrUnauthorized) {
				p.promptReconnect(userID, channelID)
				return
			}
			p.sendUserEphemeral(userID, channelID, func(t Translations) string {
				return fmt.Sprintf(t.AssignFailed, issueKey, err.Error())
			})
			return
		}

		p.enqueueForceUpdate(issueKey)
		p.sendUserEphemeral(userID, channelID, func(t Translations) string {
			return fmt.Sprintf(t.AssignedToYou, issueKey)
		})
	}()
}

// handleChangeStatus fetches available transitions and opens a status-picker dialog.
func (p *Plugin) handleChangeStatus(w http.ResponseWriter, r *http.Request, userID string) {
	var req model.PostActionIntegrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if !bodyUserMatches(req.UserId, userID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	issueKey, _ := req.Context["issue_key"].(string)
	if !validIssueKey(issueKey) || !p.isConfigured() {
		writeActionResponse(w, nil)
		return
	}

	// Transitions are fetched and executed with the user's own token so both
	// the available options and the change itself reflect their Tracker account.
	client, _ := p.getClientForUser(userID)
	if client == nil {
		p.promptConnect(userID, req.ChannelId)
		writeActionResponse(w, nil)
		return
	}

	// Copy only the fields needed by the goroutine — do not capture req directly.
	channelID := req.ChannelId
	triggerID := req.TriggerId
	postID := req.PostId

	// Respond immediately — GetTransitions is a blocking HTTP call that can exceed
	// MM's action integration timeout, which kills the RPC connection and crashes the plugin.
	writeActionResponse(w, nil)

	// Fetch transitions and open the dialog in a goroutine.
	// The trigger ID is valid for 3 seconds; if GetTransitions takes longer the
	// OpenInteractiveDialog call will fail, but the plugin will not crash.
	// wg tracks this goroutine so OnDeactivate waits for it before tearing down plugin state.
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		if p.ctx.Err() != nil {
			return
		}

		var userLocale string
		if user, appErr := p.API.GetUser(userID); appErr == nil {
			userLocale = user.Locale
		}

		transitions := p.store.GetCachedTransitions(issueKey)
		if transitions == nil {
			var err error
			transitions, err = client.GetTransitions(tracker.ContextWithLocale(p.ctx, userLocale), issueKey)
			if err != nil {
				p.API.LogError("change-status: failed to get transitions", "key", issueKey, "err", err.Error())
				if errors.Is(err, tracker.ErrUnauthorized) {
					p.promptReconnect(userID, channelID)
					return
				}
				p.API.SendEphemeralPost(userID, &model.Post{
					ChannelId: channelID,
					Message:   fmt.Sprintf("Could not load available statuses for %s. Please try again.", issueKey),
				})
				return
			}
			p.store.CacheTransitions(issueKey, transitions)
		}

		options := make([]*model.PostActionOptions, 0, len(transitions))
		for _, t := range transitions {
			options = append(options, &model.PostActionOptions{Text: t.Name, Value: t.ID})
		}

		t := translationsForLocale(userLocale)
		siteURL := p.siteURL()

		if appErr := p.API.OpenInteractiveDialog(model.OpenDialogRequest{
			TriggerId: triggerID,
			URL:       fmt.Sprintf("%s/plugins/%s/transition", siteURL, pluginID),
			Dialog: model.Dialog{
				CallbackId:  issueKey + "|" + postID,
				Title:       fmt.Sprintf(t.StatusDialogTitle, issueKey),
				SubmitLabel: t.StatusDialogSubmit,
				Elements: []model.DialogElement{
					{
						DisplayName: t.StatusFieldLabel,
						Name:        "transition_id",
						Type:        "select",
						Options:     options,
					},
				},
			},
		}); appErr != nil {
			p.API.LogError("change-status: failed to open dialog", "key", issueKey, "err", appErr.Error())
		}
	}()
}

// handleTransition receives the dialog submission and executes the chosen transition.
func (p *Plugin) handleTransition(w http.ResponseWriter, r *http.Request, userID string) {
	var req model.SubmitDialogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if !bodyUserMatches(req.UserId, userID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if req.Cancelled {
		writeSubmitDialogResponse(w, "")
		return
	}

	// CallbackId = "issueKey|postID"
	parts := strings.SplitN(req.CallbackId, "|", 2)
	if len(parts) != 2 {
		writeSubmitDialogResponse(w, "Invalid request.")
		return
	}
	issueKey, postID := parts[0], parts[1]
	if !validIssueKey(issueKey) {
		writeSubmitDialogResponse(w, "Invalid request.")
		return
	}

	transitionID, _ := req.Submission["transition_id"].(string)
	if transitionID == "" {
		writeSubmitDialogResponse(w, "Please select a status.")
		return
	}

	// Look up transition display name from cache (needed for config matching).
	transitions := p.store.GetCachedTransitions(issueKey)
	transitionName := transitionID // fallback if not cached
	for _, tr := range transitions {
		if tr.ID == transitionID {
			transitionName = tr.Name
			break
		}
	}

	// Check whether this transition requires fields to be collected upfront.
	// requiredFields: fieldName → {displayLabel: apiKey} (empty map = free text).
	cfg := p.getConfiguration()
	requiredFields := cfg.lookupRequiredFields(transitionID, transitionName)

	if len(requiredFields) > 0 {
		// Store the pending transition and prompt the user to fill the required fields.
		if err := p.store.SetPendingTransition(userID, PendingTransition{
			IssueKey:       issueKey,
			PostID:         postID,
			TransitionID:   transitionID,
			TransitionName: transitionName,
			Fields:         requiredFields,
		}); err != nil {
			p.API.LogError("transition: failed to store pending transition", "key", issueKey, "err", err.Error())
			writeSubmitDialogResponse(w, "Internal error — please try again.")
			return
		}

		var userLocale string
		if user, appErr := p.API.GetUser(userID); appErr == nil {
			userLocale = user.Locale
		}
		t := translationsForLocale(userLocale)
		siteURL := p.siteURL()

		p.API.SendEphemeralPost(userID, &model.Post{
			ChannelId: req.ChannelId,
			Props: map[string]interface{}{
				"attachments": []*model.SlackAttachment{{
					Text: fmt.Sprintf(t.FillFieldsPrompt, issueKey, transitionName),
					Actions: []*model.PostAction{{
						Name: t.FillFieldsButton,
						Type: model.PostActionTypeButton,
						Integration: &model.PostActionIntegration{
							URL: fmt.Sprintf("%s/plugins/%s/fill-fields", siteURL, pluginID),
						},
					}},
				}},
			},
		})

		writeSubmitDialogResponse(w, "")
		return
	}

	// No required fields — execute the transition directly, as the user.
	client, _ := p.getClientForUser(userID)
	if client == nil {
		writeSubmitDialogResponse(w, p.translationsForUser(userID).ConnectPrompt)
		return
	}
	if err := client.ExecuteTransition(p.ctx, issueKey, transitionID, nil); err != nil {
		p.API.LogError("transition: failed to execute transition", "key", issueKey, "userID", userID, "err", err.Error())
		writeSubmitDialogResponse(w, p.writeErrorMessage(userID, err, "Failed to change status. Please try again."))
		return
	}

	// Invalidate cached transitions — they are bound to the old status and will
	// return 404 if reused after the issue moves to a new state.
	p.store.InvalidateCachedTransitions(issueKey)
	p.enqueueForceUpdate(issueKey)
	writeSubmitDialogResponse(w, "")
}

// handleFillFields opens a dialog for the required fields of a pending transition.
// Called when the user clicks the "Fill Fields" button on the ephemeral prompt post.
func (p *Plugin) handleFillFields(w http.ResponseWriter, r *http.Request, userID string) {
	var req model.PostActionIntegrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if !bodyUserMatches(req.UserId, userID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	pending, err := p.store.GetPendingTransition(userID)
	if err != nil || pending == nil {
		p.API.LogError("fill-fields: no pending transition", "userID", userID)
		writeActionResponse(w, nil)
		return
	}

	var userLocale string
	if user, appErr := p.API.GetUser(userID); appErr == nil {
		userLocale = user.Locale
	}
	t := translationsForLocale(userLocale)
	siteURL := p.siteURL()

	// Build dialog elements from the required fields config.
	// Each field is either a select (label→apiKey pairs) or free text (empty map).
	// Sort field names and option labels for a stable, predictable dialog layout.
	fieldNames := make([]string, 0, len(pending.Fields))
	for fieldName := range pending.Fields {
		fieldNames = append(fieldNames, fieldName)
	}
	sort.Strings(fieldNames)

	elements := make([]model.DialogElement, 0, len(fieldNames))
	for _, fieldName := range fieldNames {
		labelToKey := pending.Fields[fieldName]
		el := model.DialogElement{
			DisplayName: fieldName,
			Name:        fieldName,
		}
		if len(labelToKey) > 0 {
			el.Type = "select"
			labels := make([]string, 0, len(labelToKey))
			for label := range labelToKey {
				labels = append(labels, label)
			}
			sort.Strings(labels)
			opts := make([]*model.PostActionOptions, 0, len(labels))
			for _, label := range labels {
				opts = append(opts, &model.PostActionOptions{Text: label, Value: labelToKey[label]})
			}
			el.Options = opts
		} else {
			el.Type = "text"
		}
		elements = append(elements, el)
	}

	if appErr := p.API.OpenInteractiveDialog(model.OpenDialogRequest{
		TriggerId: req.TriggerId,
		URL:       fmt.Sprintf("%s/plugins/%s/fill-fields-submit", siteURL, pluginID),
		Dialog: model.Dialog{
			CallbackId:  pending.IssueKey + "|" + pending.PostID,
			Title:       fmt.Sprintf(t.FillFieldsDialogTitle, pending.IssueKey, pending.TransitionName),
			SubmitLabel: t.FillFieldsDialogSubmit,
			Elements:    elements,
		},
	}); appErr != nil {
		p.API.LogError("fill-fields: failed to open dialog", "key", pending.IssueKey, "err", appErr.Error())
	}

	writeActionResponse(w, nil)
}

// handleFillFieldsSubmit receives the filled-fields dialog, executes the pending transition,
// and cleans up the KV entry.
func (p *Plugin) handleFillFieldsSubmit(w http.ResponseWriter, r *http.Request, userID string) {
	var req model.SubmitDialogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if !bodyUserMatches(req.UserId, userID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if req.Cancelled {
		p.store.DeletePendingTransition(userID)
		writeSubmitDialogResponse(w, "")
		return
	}

	pending, err := p.store.GetPendingTransition(userID)
	if err != nil || pending == nil {
		writeSubmitDialogResponse(w, "Session expired — please use Change Status again.")
		return
	}

	// Collect submitted field values.
	// Select fields: the submitted value is the apiKey chosen by the user.
	//   Yandex Tracker expects reference fields as {"key": apiKey}.
	// Free-text fields (empty labelToKey map): sent as plain strings.
	fields := make(map[string]interface{}, len(req.Submission))
	for k, v := range req.Submission {
		s, ok := v.(string)
		if !ok {
			continue
		}
		if labelToKey, exists := pending.Fields[k]; exists && len(labelToKey) > 0 {
			fields[k] = map[string]interface{}{"key": s}
		} else {
			fields[k] = s
		}
	}

	client, _ := p.getClientForUser(userID)
	if client == nil {
		writeSubmitDialogResponse(w, p.translationsForUser(userID).ConnectPrompt)
		return
	}
	if err := client.ExecuteTransition(p.ctx, pending.IssueKey, pending.TransitionID, fields); err != nil {
		p.API.LogError("fill-fields-submit: failed to execute transition",
			"key", pending.IssueKey, "transitionID", pending.TransitionID, "userID", userID, "err", err.Error())
		writeSubmitDialogResponse(w, p.writeErrorMessage(userID, err, "Failed to change status. Please try again."))
		return
	}

	p.store.DeletePendingTransition(userID)
	p.store.InvalidateCachedTransitions(pending.IssueKey)
	p.enqueueForceUpdate(pending.IssueKey)
	writeSubmitDialogResponse(w, "")
}

// handleAddComment receives a postID + issueKey from the webapp, fetches the post text,
// and posts it as a comment on the Tracker issue attributed to the MM user.
func (p *Plugin) handleAddComment(w http.ResponseWriter, r *http.Request, userID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		PostID   string `json:"post_id"`
		IssueKey string `json:"issue_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, map[string]interface{}{"ok": false, "error": "bad request"})
		return
	}

	req.IssueKey = strings.TrimSpace(strings.ToUpper(req.IssueKey))
	if req.PostID == "" || !validIssueKey(req.IssueKey) {
		respondJSON(w, map[string]interface{}{"ok": false, "error": "post_id and a valid issue_key are required"})
		return
	}

	if !p.isConfigured() {
		respondJSON(w, map[string]interface{}{"ok": false, "error": "plugin is not configured"})
		return
	}

	// Comments are posted with the user's own token so Tracker shows them as
	// the real author — no service-account attribution, no "(via …)" suffix.
	client, _ := p.getClientForUser(userID)
	if client == nil {
		respondJSON(w, map[string]interface{}{"ok": false, "error": p.translationsForUser(userID).ConnectPrompt})
		return
	}

	post, appErr := p.API.GetPost(req.PostID)
	if appErr != nil {
		p.API.LogError("add-comment: failed to get post", "postID", req.PostID, "err", appErr.Error())
		respondJSON(w, map[string]interface{}{"ok": false, "error": "could not load message"})
		return
	}
	// Only allow commenting content the caller can actually see — a post ID from
	// a channel the user is not a member of must not be exfiltrated to Tracker.
	if !p.canReadPost(userID, post) {
		respondJSON(w, map[string]interface{}{"ok": false, "error": "you do not have access to that message"})
		return
	}

	if err := client.AddComment(p.ctx, req.IssueKey, post.Message); err != nil {
		p.API.LogError("add-comment: failed to add comment", "key", req.IssueKey, "userID", userID, "err", err.Error())
		respondJSON(w, map[string]interface{}{"ok": false, "error": p.writeErrorMessage(userID, err, "Could not add the comment. Please try again.")})
		return
	}

	respondJSON(w, map[string]interface{}{"ok": true})
}

// writeSubmitDialogResponse writes a SubmitDialogResponse. errMsg="" signals success.
func writeSubmitDialogResponse(w http.ResponseWriter, errMsg string) {
	resp := model.SubmitDialogResponse{}
	if errMsg != "" {
		resp.Error = errMsg
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func writeActionResponse(w http.ResponseWriter, resp *model.PostActionIntegrationResponse) {
	if resp == nil {
		resp = &model.PostActionIntegrationResponse{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
