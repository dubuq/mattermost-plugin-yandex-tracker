package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// handleWebhookURL returns the full webhook URL for display in the System Console.
func (p *Plugin) handleWebhookURL(w http.ResponseWriter, r *http.Request) {
	siteURL := ""
	if cfg := p.API.GetConfig(); cfg.ServiceSettings.SiteURL != nil {
		siteURL = strings.TrimRight(*cfg.ServiceSettings.SiteURL, "/")
	}

	url := fmt.Sprintf("%s/plugins/%s/webhook", siteURL, pluginID)
	respondJSON(w, map[string]string{"url": url})
}

// handleVerify tests credentials by calling the tracker /myself endpoint.
func (p *Plugin) handleVerify(w http.ResponseWriter, r *http.Request) {
	if !p.isConfigured() {
		respondJSON(w, map[string]interface{}{
			"ok":    false,
			"error": "Plugin is not configured — set the token and org ID first.",
		})
		return
	}

	if err := p.getTrackerClient().Ping(p.ctx); err != nil {
		respondJSON(w, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}

	respondJSON(w, map[string]interface{}{"ok": true})
}

// handleClearCache deletes all post-related KV entries while preserving subscriptions and user logins.
func (p *Plugin) handleClearCache(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	deleted, err := p.store.ClearPostCache()
	if err != nil {
		respondJSON(w, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}

	respondJSON(w, map[string]interface{}{"ok": true, "deleted": deleted})
}

func respondJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
