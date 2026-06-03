CREATE TABLE slack_integrations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id        TEXT NOT NULL UNIQUE REFERENCES agents(id) ON DELETE CASCADE,
    bot_token       TEXT NOT NULL,
    app_token       TEXT NOT NULL,
    enabled         BOOLEAN NOT NULL DEFAULT FALSE,
    workspace_id    TEXT NOT NULL DEFAULT '',
    workspace_name  TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'disconnected',
    status_message  TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_slack_integrations_agent ON slack_integrations(agent_id);
CREATE INDEX idx_slack_integrations_enabled ON slack_integrations(enabled) WHERE enabled = TRUE;
