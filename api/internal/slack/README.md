# Slack Integration

Slack Bot integration for Agentgram agents using Socket Mode.

## Slack App Configuration

### 1. Create the Slack App

Go to https://api.slack.com/apps and create a new app. Use the following minimal manifest:

```yaml
display_information:
  name: Agentgram Bot
  description: Bot de Agentgram AI Agent
oauth_config:
  scopes:
    bot:
      - chat:write
      - channels:history
      - groups:history
      - im:history
      - im:write
      - users:read
      - users:read.email
      - app_mentions:read
settings:
  socket_mode_enabled: true
  event_subscriptions:
    bot_events:
      - message.im
      - app_mention
  org_deploy_enabled: false
```

### 2. Obtain tokens

1. **Bot Token** (`xoxb-...`): OAuth & Permissions > Bot User OAuth Token
2. **App Token** (`xapp-...`): Basic Information > App-Level Tokens > Create a token with the `connections:write` scope

### 3. Configure in Agentgram

1. Go to Admin > Agents > [your agent] > Integrations
2. Enable "Slack integration"
3. Enter the Bot Token and App Token
4. Test the connection
5. Save

### Required scopes

| Scope | Use |
|-------|-----|
| `chat:write` | Send bot responses |
| `channels:history` | Read thread history in channels |
| `groups:history` | Read history in private channels |
| `im:history` | Read direct message history |
| `im:write` | Send direct messages |
| `users:read` | Read user info |
| `users:read.email` | Resolve email for permissions |
| `app_mentions:read` | Receive @bot mentions |

### Subscribed events

| Event | Use |
|--------|-----|
| `message.im` | Direct messages to the bot |
| `app_mention` | @bot mentions in channels |

## Architecture

- Each agent can have its own Slack bot
- Socket Mode: no public HTTP endpoints
- Multi-pod: all pods connect, with per-event dedup in Redis
- Progressive responses: chat.update with a 500ms debounce
- Tool calls: a compact counter that updates
- Auth: Slack email → Keycloak groups → HasAccessWithInherited()
