package main

import (
	"fmt"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/dubuq/mattermost-plugin-yandex-tracker/server/tracker"
)

// ExecuteCommand handles /tracker slash commands:
//
//	/tracker PROJECT-1            — fetch issue card (ephemeral)
//	/tracker subscribe QUEUE      — subscribe this channel to a queue
//	/tracker unsubscribe QUEUE    — remove the subscription
//	/tracker subscriptions        — list this channel's subscriptions
func (p *Plugin) ExecuteCommand(_ *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	input := strings.TrimSpace(strings.TrimPrefix(args.Command, "/tracker"))

	subcommand, rest := splitFirst(input)

	switch subcommand {
	case "subscribe":
		return p.cmdSubscribe(args, rest)
	case "unsubscribe":
		return p.cmdUnsubscribe(args, rest)
	case "subscriptions":
		return p.cmdListSubscriptions(args)
	default:
		return p.cmdFetchIssue(args, input)
	}
}

func (p *Plugin) cmdFetchIssue(args *model.CommandArgs, input string) (*model.CommandResponse, *model.AppError) {
	if input == "" {
		return ephemeralText("Usage: `/tracker PROJECT-1`"), nil
	}

	// Normalise: strip a full tracker URL down to the key if the user pasted one.
	if match := issueKeyPattern.FindString(input); match != "" {
		input = match
	}

	if !p.isConfigured() {
		return ephemeralText("Yandex Tracker plugin is not configured. Ask your admin to set the token and org ID."), nil
	}

	cfg := p.getConfiguration()
	issue, err := p.getTrackerClient().GetIssue(tracker.ContextWithLocale(p.ctx, p.serverLocale()), input)
	if err != nil {
		p.API.LogError("command: failed to fetch issue", "key", input, "userID", args.UserId, "err", err.Error())
		return ephemeralText("Could not fetch issue `" + input + "`. Make sure the key is correct and the token has access."), nil
	}

	// Slash command cards start expanded — the user explicitly asked to see this issue.
	attachment := p.formatter.BuildAttachment(issue, cfg.resolveStatusColor(issue.Status), p.translations(), false)
	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Attachments:  []*model.SlackAttachment{attachment},
	}, nil
}

func (p *Plugin) cmdSubscribe(args *model.CommandArgs, queueKey string) (*model.CommandResponse, *model.AppError) {
	queueKey = strings.ToUpper(strings.TrimSpace(queueKey))
	if queueKey == "" {
		return ephemeralText("Usage: `/tracker subscribe QUEUE` — e.g. `/tracker subscribe DEV`"), nil
	}
	if err := p.store.AddSubscription(args.ChannelId, queueKey); err != nil {
		p.API.LogError("command: subscribe failed", "queue", queueKey, "channelID", args.ChannelId, "err", err.Error())
		return ephemeralText(fmt.Sprintf("Failed to subscribe to queue %s. Please try again.", queueKey)), nil
	}
	return ephemeralText(fmt.Sprintf("This channel is now subscribed to **%s**. New issues will be posted here automatically.", queueKey)), nil
}

func (p *Plugin) cmdUnsubscribe(args *model.CommandArgs, queueKey string) (*model.CommandResponse, *model.AppError) {
	queueKey = strings.ToUpper(strings.TrimSpace(queueKey))
	if queueKey == "" {
		return ephemeralText("Usage: `/tracker unsubscribe QUEUE` — e.g. `/tracker unsubscribe DEV`"), nil
	}
	if err := p.store.RemoveSubscription(args.ChannelId, queueKey); err != nil {
		p.API.LogError("command: unsubscribe failed", "queue", queueKey, "channelID", args.ChannelId, "err", err.Error())
		return ephemeralText(fmt.Sprintf("Failed to unsubscribe from queue %s. Please try again.", queueKey)), nil
	}
	return ephemeralText(fmt.Sprintf("This channel is no longer subscribed to **%s**.", queueKey)), nil
}

func (p *Plugin) cmdListSubscriptions(args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	queues, err := p.store.GetChannelQueues(args.ChannelId)
	if err != nil {
		p.API.LogError("command: list subscriptions failed", "channelID", args.ChannelId, "err", err.Error())
		return ephemeralText("Failed to load subscriptions. Please try again."), nil
	}
	if len(queues) == 0 {
		return ephemeralText("This channel has no queue subscriptions. Use `/tracker subscribe QUEUE` to add one."), nil
	}
	lines := []string{"**Queue subscriptions for this channel:**"}
	for _, q := range queues {
		lines = append(lines, "- "+q)
	}
	return ephemeralText(strings.Join(lines, "\n")), nil
}

func ephemeralText(text string) *model.CommandResponse {
	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         text,
	}
}

// splitFirst splits s into the first word and the rest, trimmed.
func splitFirst(s string) (first, rest string) {
	s = strings.TrimSpace(s)
	idx := strings.IndexByte(s, ' ')
	if idx < 0 {
		return s, ""
	}
	return s[:idx], strings.TrimSpace(s[idx+1:])
}
