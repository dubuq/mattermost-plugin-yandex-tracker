package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
)

// Configuration holds all admin-configurable settings, populated by MM from System Console.
type Configuration struct {
	TrackerToken       string `json:"TrackerToken"`
	OrgID              string `json:"OrgID"`
	WebhookSecret      string `json:"WebhookSecret"`
	BotDisplayName     string `json:"BotDisplayName"`
	MonitorAllChannels      bool   `json:"MonitorAllChannels"`
	BackgroundRefreshHours string `json:"BackgroundRefreshHours"` // "0"=disabled; otherwise interval in hours

	// Card sidebar colors — all optional, fall back to built-in defaults when blank.
	ColorActive    string `json:"ColorActive"`
	ColorDone      string `json:"ColorDone"`
	ColorCancelled string `json:"ColorCancelled"`
	ColorDefault   string `json:"ColorDefault"`
	ColorCustom    string `json:"ColorCustom"` // optional 5th color slot

	// Comma-separated lists of status names that map to each color slot.
	// Matching is case-insensitive. Anything not listed falls back to ColorDefault.
	StatusesActive    string `json:"StatusesActive"`
	StatusesDone      string `json:"StatusesDone"`
	StatusesCancelled string `json:"StatusesCancelled"`
	StatusesCustom    string `json:"StatusesCustom"` // used only when ColorCustom is also set

	// RequiredFieldsByTransition is a JSON object that maps transition names to the fields
	// that must be collected before executing them. Format:
	//   { "TransitionName": { "fieldName": ["option1", "option2"] } }
	// Non-empty array → select dropdown; empty array → free-text input.
	RequiredFieldsByTransition string `json:"RequiredFieldsByTransition"`
}

// parsedRequiredFields parses RequiredFieldsByTransition into a usable map.
//
// Config format:
//
//	{
//	  "TransitionName": {
//	    "fieldName": { "Display Label": "apiKey", ... },  // select: label→key pairs
//	    "freeTextField": {}                                // free text: empty object
//	  }
//	}
//
// Returns nil if the config is blank or not valid JSON.
func (c *Configuration) parsedRequiredFields() map[string]map[string]map[string]string {
	s := strings.TrimSpace(c.RequiredFieldsByTransition)
	if s == "" {
		return nil
	}
	var result map[string]map[string]map[string]string
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		return nil
	}
	return result
}

// lookupRequiredFields returns the required-field config for a transition, matched
// case-insensitively against both the transition's ID and its display name.
// Returns nil if no entry matches.
func (c *Configuration) lookupRequiredFields(transitionID, transitionName string) map[string]map[string]string {
	for configKey, fields := range c.parsedRequiredFields() {
		if strings.EqualFold(configKey, transitionID) || strings.EqualFold(configKey, transitionName) {
			return fields
		}
	}
	return nil
}

// statusColors returns configured colors with built-in defaults as fallback.
func (c *Configuration) statusColors() StatusColors {
	pick := func(configured, fallback string) string {
		if configured != "" {
			return configured
		}
		return fallback
	}
	return StatusColors{
		Active:    pick(c.ColorActive, "#1E88E5"),
		Done:      pick(c.ColorDone, "#43A047"),
		Cancelled: pick(c.ColorCancelled, "#E53935"),
		Default:   pick(c.ColorDefault, "#AAAAAA"),
	}
}

// resolveStatusColor maps a status string to its configured hex color.
// Falls back to Default for any status not matched by a list.
func (c *Configuration) resolveStatusColor(status string) string {
	colors := c.statusColors()
	if statusInList(status, c.StatusesActive) {
		return colors.Active
	}
	if statusInList(status, c.StatusesDone) {
		return colors.Done
	}
	if statusInList(status, c.StatusesCancelled) {
		return colors.Cancelled
	}
	if c.ColorCustom != "" && statusInList(status, c.StatusesCustom) {
		return c.ColorCustom
	}
	return colors.Default
}

// statusInList reports whether status appears in a comma-separated list.
// Comparison is case-insensitive and trims surrounding whitespace.
func statusInList(status, list string) bool {
	for _, s := range splitTrimmed(list, ",") {
		if strings.EqualFold(s, status) {
			return true
		}
	}
	return false
}

// refreshIntervalHours parses the configured interval string.
// Returns 0 if disabled, defaults to 6 if blank or unparseable.
func (c *Configuration) refreshIntervalHours() int {
	if c.BackgroundRefreshHours == "0" {
		return 0
	}
	n, err := strconv.Atoi(c.BackgroundRefreshHours)
	if err != nil || n < 1 {
		return 6 // default
	}
	return n
}

func (c *Configuration) isValid() error {
	if c.TrackerToken == "" {
		return fmt.Errorf("tracker token is required")
	}
	if c.OrgID == "" {
		return fmt.Errorf("organization ID is required")
	}
	return nil
}

// clone returns a shallow copy so callers can't mutate shared state.
func (c *Configuration) clone() *Configuration {
	clone := *c
	return &clone
}

// getConfiguration returns a thread-safe snapshot of the current configuration.
func (p *Plugin) getConfiguration() *Configuration {
	p.configLock.RLock()
	defer p.configLock.RUnlock()

	if p.configuration == nil {
		return &Configuration{}
	}

	return p.configuration.clone()
}

func (p *Plugin) setConfiguration(c *Configuration) {
	p.configLock.Lock()
	defer p.configLock.Unlock()
	p.configuration = c
}

func (p *Plugin) OnConfigurationChange() error {
	var cfg Configuration
	if err := p.API.LoadPluginConfiguration(&cfg); err != nil {
		return fmt.Errorf("failed to load plugin configuration: %w", err)
	}

	p.setConfiguration(&cfg)
	p.initTrackerClient()

	// botUserID is only set after OnActivate completes its init sequence,
	// so this guard prevents double-starts and premature bot updates during
	// the initial activation.
	if p.botUserID != "" {
		// Keep the bot's display name in sync with the configured value.
		// EnsureBotUser is idempotent and updates the existing bot when called again.
		if _, appErr := p.API.EnsureBotUser(&model.Bot{
			Username:    "tracker-bot",
			DisplayName: cfg.BotDisplayName,
			Description: "Yandex Tracker integration bot",
		}); appErr != nil {
			p.API.LogWarn("OnConfigurationChange: failed to update bot display name", "err", appErr.Error())
		}

		p.stopBackgroundRefresh()
		p.startBackgroundRefresh()
	}

	if err := cfg.isValid(); err != nil {
		p.API.LogWarn("Plugin configuration is incomplete — hook will not run", "reason", err.Error())
	}

	if cfg.WebhookSecret == "" {
		p.API.LogWarn("WebhookSecret is not set — webhook endpoint accepts all requests. Set a secret before going to production.")
	}

	return nil
}
