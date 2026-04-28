package main

import (
	"net/http"

	"github.com/mattermost/mattermost/server/public/plugin"
)

// ServeHTTP routes plugin HTTP requests. MM forwards /plugins/{plugin-id}/{path} here with r.URL.Path = /{path}.
func (p *Plugin) ServeHTTP(_ *plugin.Context, w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/webhook":
		p.safeServe("webhook", p.handleWebhook)(w, r)
	case "/refresh":
		p.safeServe("refresh", p.handleRefresh)(w, r)
	case "/dismiss":
		p.safeServe("dismiss", p.handleDismiss)(w, r)
	case "/collapse":
		p.safeServe("collapse", p.handleCollapse)(w, r)
	case "/expand":
		p.safeServe("expand", p.handleExpand)(w, r)
	case "/assign":
		p.safeServe("assign", p.handleAssignToMe)(w, r)
	case "/assign-submit":
		p.safeServe("assign-submit", p.handleAssignSubmit)(w, r)
	case "/change-status":
		p.safeServe("change-status", p.handleChangeStatus)(w, r)
	case "/transition":
		p.safeServe("transition", p.handleTransition)(w, r)
	case "/webhook-url":
		p.safeServe("webhook-url", p.handleWebhookURL)(w, r)
	case "/verify":
		p.safeServe("verify", p.handleVerify)(w, r)
	case "/clear-cache":
		p.safeServe("clear-cache", p.handleClearCache)(w, r)
	case "/add-comment":
		p.safeServe("add-comment", p.handleAddComment)(w, r)
	case "/fill-fields":
		p.safeServe("fill-fields", p.handleFillFields)(w, r)
	case "/fill-fields-submit":
		p.safeServe("fill-fields-submit", p.handleFillFieldsSubmit)(w, r)
	default:
		http.NotFound(w, r)
	}
}
