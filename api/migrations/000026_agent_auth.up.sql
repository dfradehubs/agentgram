-- Agent outbound auth: adds auth_type (none|forward|bearer, oauth2 reserved),
-- bearer token fallback, configurable auth header, and per user/group API key rules.

ALTER TABLE agents
    ADD COLUMN auth_type        TEXT NOT NULL DEFAULT '',
    ADD COLUMN bearer_token     TEXT NOT NULL DEFAULT '',
    ADD COLUMN auth_header_name TEXT NOT NULL DEFAULT '';

-- Migrate existing forward_authorization agents to auth_type='forward'
UPDATE agents SET auth_type = 'forward' WHERE forward_authorization = true;

-- API key rules: maps a user email or group to the API key agentgram sends to
-- the agent in bearer mode. Resolution order: user match > group match (by
-- position) > agents.bearer_token fallback.
CREATE TABLE agent_api_key_rules (
    id           TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    agent_id     VARCHAR(255) NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    subject_type TEXT NOT NULL CHECK (subject_type IN ('user', 'group')),
    subject      TEXT NOT NULL,
    api_key      TEXT NOT NULL,
    position     INT NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(agent_id, subject_type, subject)
);

CREATE INDEX idx_agent_api_key_rules_agent ON agent_api_key_rules(agent_id);
