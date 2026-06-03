package slack

import (
	"context"
	"strings"

	slackapi "github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
	"go.uber.org/zap"
)

// StatusCallback is called when a bot's connection status changes.
type StatusCallback func(agentID, status, message string)

// Bot represents a single Slack bot connection via Socket Mode.
type Bot struct {
	agentID      string
	client       *slackapi.Client
	socketClient *socketmode.Client
	handler      *MessageHandler
	botUserID    string
	onStatus     StatusCallback
	logger       *zap.Logger
	cancel       context.CancelFunc
	done         chan struct{}
}

// NewBot creates a new Bot for a given agent.
func NewBot(agentID, botToken, appToken string, handler *MessageHandler, onStatus StatusCallback, logger *zap.Logger) *Bot {
	client := slackapi.New(botToken, slackapi.OptionAppLevelToken(appToken))
	socketClient := socketmode.New(client)

	return &Bot{
		agentID:      agentID,
		client:       client,
		socketClient: socketClient,
		handler:      handler,
		onStatus:     onStatus,
		logger:       logger.With(zap.String("agent_id", agentID)),
		done:         make(chan struct{}),
	}
}

// Start begins the Socket Mode event loop in a new goroutine.
func (b *Bot) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	b.cancel = cancel

	// Resolve bot's own user ID to filter self-messages
	if resp, err := b.client.AuthTest(); err == nil {
		b.botUserID = resp.UserID
		b.logger.Info("slack bot authenticated", zap.String("bot_user_id", b.botUserID))
	} else {
		b.logger.Warn("failed to resolve bot user ID", zap.Error(err))
	}

	// Event handler goroutine
	go func() {
		defer close(b.done)
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-b.socketClient.Events:
				if !ok {
					return
				}
				b.handleSocketEvent(ctx, evt)
			}
		}
	}()

	// Socket Mode connection goroutine
	go func() {
		if err := b.socketClient.RunContext(ctx); err != nil {
			if ctx.Err() == nil {
				b.logger.Error("socket mode disconnected", zap.Error(err))
				b.reportStatus("error", err.Error())
			}
		}
	}()
}

// Stop gracefully shuts down the bot.
func (b *Bot) Stop() {
	if b.cancel != nil {
		b.cancel()
	}
	<-b.done
	b.reportStatus("disconnected", "")
}

func (b *Bot) handleSocketEvent(ctx context.Context, evt socketmode.Event) {
	b.logger.Debug("socket event received", zap.String("type", string(evt.Type)))

	switch evt.Type {
	case socketmode.EventTypeEventsAPI:
		eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
		if !ok {
			b.logger.Warn("failed to cast events API event", zap.Any("data", evt.Data))
			return
		}
		b.socketClient.Ack(*evt.Request)
		b.logger.Debug("events API event received",
			zap.String("event_type", eventsAPIEvent.Type),
			zap.String("inner_type", eventsAPIEvent.InnerEvent.Type))
		b.handleEventsAPI(ctx, eventsAPIEvent)

	case socketmode.EventTypeConnectionError:
		b.logger.Warn("socket mode connection error", zap.Any("data", evt.Data))
		b.reportStatus("error", "connection error")

	case socketmode.EventTypeConnected:
		b.logger.Info("socket mode connected")
		b.reportStatus("connected", "")

	default:
		b.logger.Debug("unhandled socket event type", zap.String("type", string(evt.Type)))
	}
}

func (b *Bot) handleEventsAPI(ctx context.Context, event slackevents.EventsAPIEvent) {
	innerType := event.InnerEvent.Type
	b.logger.Info("handling inner event", zap.String("inner_type", innerType), zap.String("expected_mention", string(slackevents.AppMention)), zap.String("expected_message", string(slackevents.Message)))

	switch innerType {
	case string(slackevents.AppMention):
		ev, ok := event.InnerEvent.Data.(*slackevents.AppMentionEvent)
		if !ok {
			b.logger.Warn("failed to cast app mention event")
			return
		}
		b.handleAppMention(ctx, ev)

	case string(slackevents.Message):
		ev, ok := event.InnerEvent.Data.(*slackevents.MessageEvent)
		if !ok {
			b.logger.Warn("failed to cast message event")
			return
		}
		b.handleDirectMessage(ctx, ev)

	default:
		b.logger.Debug("unhandled inner event type", zap.String("type", innerType))
	}
}

func (b *Bot) handleAppMention(ctx context.Context, ev *slackevents.AppMentionEvent) {
	if b.isSelfMessage(ev.User) {
		return
	}

	// Strip ALL bot mentions from text (not just own — handles multi-mention)
	text := stripMentions(ev.Text)
	text = strings.TrimSpace(text)

	threadTS := ev.ThreadTimeStamp
	if threadTS == "" {
		threadTS = ev.TimeStamp
	}

	// Empty mention outside a thread — nothing to do
	if text == "" && threadTS == ev.TimeStamp {
		return
	}

	SlackEventsTotal.WithLabelValues(b.agentID, "app_mention").Inc()
	b.handler.HandleMessage(ctx, b.client, b.agentID, ev.TimeStamp, ev.User, ev.Channel, threadTS, text)
}

func (b *Bot) handleDirectMessage(ctx context.Context, ev *slackevents.MessageEvent) {
	// Only handle DMs (channel type "im" or channel starts with "D")
	if ev.ChannelType != "im" && !strings.HasPrefix(ev.Channel, "D") {
		return
	}
	if b.isSelfMessage(ev.User) {
		return
	}
	// Ignore subtypes (edits, deletes, bot_message, etc.)
	if ev.SubType != "" {
		return
	}
	if strings.TrimSpace(ev.Text) == "" {
		return
	}

	threadTS := ev.ThreadTimeStamp
	if threadTS == "" {
		threadTS = ev.TimeStamp
	}

	SlackEventsTotal.WithLabelValues(b.agentID, "direct_message").Inc()
	b.handler.HandleMessage(ctx, b.client, b.agentID, ev.TimeStamp, ev.User, ev.Channel, threadTS, ev.Text)
}

func (b *Bot) isSelfMessage(userID string) bool {
	return b.botUserID != "" && userID == b.botUserID
}

func (b *Bot) reportStatus(status, message string) {
	if b.onStatus != nil {
		b.onStatus(b.agentID, status, message)
	}
}
