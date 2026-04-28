package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/dubuq/mattermost-plugin-yandex-tracker/server/tracker"
	"github.com/dubuq/mattermost-plugin-yandex-tracker/server/tracker/yandex"
)

const (
	pluginID                 = "com.yandex-tracker-mattermost"
	trackerLinkPreviewDomain = "tracker.yandex.ru"
)

type Plugin struct {
	plugin.MattermostPlugin

	configLock    sync.RWMutex
	configuration *Configuration
	trackerClient tracker.Client

	botUserID string // written once in OnActivate before handlers start; safe to read concurrently
	store     *Store
	formatter *Formatter

	// ctx is cancelled in OnDeactivate to signal all goroutines to stop.
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// updateCh is the single queue for all card updates.
	// One goroutine drains it — the only goroutine that calls UpdatePost.
	updateCh chan updateJob

	// refreshMu protects refreshCancel so startBackgroundRefresh / stopBackgroundRefresh
	// can be called from OnConfigurationChange without a race.
	refreshMu     sync.Mutex
	refreshCancel context.CancelFunc
}

func (p *Plugin) OnActivate() error {
	// Set up the plugin-lifetime context and update channel before calling
	// OnConfigurationChange so goroutines started by config change are safe.
	p.ctx, p.cancel = context.WithCancel(context.Background())
	p.updateCh = make(chan updateJob, 64)

	if err := p.OnConfigurationChange(); err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	botID, err := p.API.EnsureBotUser(&model.Bot{
		Username:    "tracker-bot",
		DisplayName: p.botDisplayName(),
		Description: "Yandex Tracker integration bot",
	})
	if err != nil {
		return fmt.Errorf("failed to ensure bot user: %w", err)
	}
	p.botUserID = botID
	p.ensureBotInAllTeams()

	p.store = NewStore(p.API)
	p.formatter = NewFormatter(pluginID)
	p.restrictTrackerLinkPreview()

	if err := p.API.RegisterCommand(&model.Command{
		Trigger:          "tracker",
		DisplayName:      "Yandex Tracker",
		Description:      "Yandex Tracker integration — fetch issues, manage queue subscriptions.",
		AutoComplete:     true,
		AutoCompleteDesc: "Fetch an issue card, or manage queue subscriptions for this channel.",
		AutoCompleteHint: "[issue key | subscribe QUEUE | unsubscribe QUEUE | subscriptions]",
	}); err != nil {
		p.API.LogWarn("Failed to register /tracker command", "err", err.Error())
	}

	p.startUpdateWorker()
	p.startBackgroundRefresh()
	p.API.LogInfo("Yandex Tracker plugin started", "botUserID", p.botUserID)
	return nil
}

// OnDeactivate cancels the plugin context and waits for all goroutines to exit.
// Nothing in the plugin should call p.API after this returns.
func (p *Plugin) OnDeactivate() error {
	p.cancel()
	p.wg.Wait()
	p.unrestrictTrackerLinkPreview()
	p.API.LogInfo("Yandex Tracker plugin stopped")
	return nil
}

// startUpdateWorker launches the single goroutine that owns all UpdatePost calls.
func (p *Plugin) startUpdateWorker() {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for {
			select {
			case job, ok := <-p.updateCh:
				if !ok {
					return
				}
				p.safeProcessJob(job)
			case <-p.ctx.Done():
				return
			}
		}
	}()
}

func (p *Plugin) restrictTrackerLinkPreview() {
	cfg := p.API.GetConfig()
	if cfg == nil {
		return
	}
	current := ""
	if cfg.ServiceSettings.RestrictLinkPreviews != nil {
		current = *cfg.ServiceSettings.RestrictLinkPreviews
	}
	domains := splitTrimmed(current, ",")
	for _, d := range domains {
		if d == trackerLinkPreviewDomain {
			return
		}
	}
	domains = append(domains, trackerLinkPreviewDomain)
	updated := strings.Join(domains, ",")
	cfg.ServiceSettings.RestrictLinkPreviews = &updated
	if appErr := p.API.SaveConfig(cfg); appErr != nil {
		p.API.LogWarn("Failed to restrict tracker link previews", "err", appErr.Error())
	}
}

func (p *Plugin) unrestrictTrackerLinkPreview() {
	cfg := p.API.GetConfig()
	if cfg == nil {
		return
	}
	current := ""
	if cfg.ServiceSettings.RestrictLinkPreviews != nil {
		current = *cfg.ServiceSettings.RestrictLinkPreviews
	}
	domains := splitTrimmed(current, ",")
	filtered := make([]string, 0, len(domains))
	for _, d := range domains {
		if d != trackerLinkPreviewDomain {
			filtered = append(filtered, d)
		}
	}
	updated := strings.Join(filtered, ",")
	cfg.ServiceSettings.RestrictLinkPreviews = &updated
	if appErr := p.API.SaveConfig(cfg); appErr != nil {
		p.API.LogWarn("Failed to unrestrict tracker link previews", "err", appErr.Error())
	}
}

func splitTrimmed(s, sep string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, sep)
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			result = append(result, t)
		}
	}
	return result
}

// ensureBotInAllTeams adds the bot user to every team so it can be manually
// invited to individual channels when MonitorAllChannels is disabled.
// Errors are logged but not fatal — the plugin works without team membership
// as long as MonitorAllChannels is true.
func (p *Plugin) ensureBotInAllTeams() {
	teams, appErr := p.API.GetTeams()
	if appErr != nil {
		p.API.LogWarn("ensureBotInAllTeams: failed to list teams", "err", appErr.Error())
		return
	}
	for _, team := range teams {
		if _, err := p.API.CreateTeamMember(team.Id, p.botUserID); err != nil {
			// Already a member or a transient error — both are acceptable.
			p.API.LogDebug("ensureBotInAllTeams: skipped", "teamID", team.Id, "err", err.Error())
		}
	}
}

func (p *Plugin) botDisplayName() string {
	if name := p.getConfiguration().BotDisplayName; name != "" {
		return name
	}
	return "Tracker Bot"
}

func (p *Plugin) isConfigured() bool {
	return p.getConfiguration().isValid() == nil && p.getTrackerClient() != nil
}

func (p *Plugin) getTrackerClient() tracker.Client {
	p.configLock.RLock()
	defer p.configLock.RUnlock()
	return p.trackerClient
}

// serverLocale returns the Mattermost server's configured default client locale (e.g. "ru", "en").
func (p *Plugin) serverLocale() string {
	cfg := p.API.GetConfig()
	if cfg != nil && cfg.LocalizationSettings.DefaultClientLocale != nil {
		return *cfg.LocalizationSettings.DefaultClientLocale
	}
	return "en"
}

func (p *Plugin) siteURL() string {
	cfg := p.API.GetConfig()
	if cfg == nil || cfg.ServiceSettings.SiteURL == nil {
		return ""
	}
	return strings.TrimRight(*cfg.ServiceSettings.SiteURL, "/")
}

// safeServe wraps a handler with a top-level recover so a panicking handler
// does not kill the plugin process.
func (p *Plugin) safeServe(name string, fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rc := recover(); rc != nil {
				// Inner recover guards against LogError itself panicking when RPC is torn down.
				func() {
					defer func() { recover() }()
					p.API.LogError("handler panicked", "handler", name, "err", fmt.Sprintf("%v", rc))
				}()
				writeActionResponse(w, nil)
			}
		}()
		fn(w, r)
	}
}

func (p *Plugin) initTrackerClient() {
	cfg := p.getConfiguration()
	var client tracker.Client
	if cfg.TrackerToken != "" && cfg.OrgID != "" {
		client = yandex.New(cfg.TrackerToken, cfg.OrgID)
	}
	p.configLock.Lock()
	p.trackerClient = client
	p.configLock.Unlock()
}

func main() {
	plugin.ClientMain(&Plugin{})
}
