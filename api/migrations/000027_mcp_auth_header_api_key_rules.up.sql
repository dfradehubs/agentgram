-- MCP server outbound auth parity with agents: configurable auth header and
-- per user/group API key rules (bearer mode).

ALTER TABLE mcp_servers
    ADD COLUMN auth_header_name TEXT NOT NULL DEFAULT '';

-- API key rules: maps a user email or group to the API key agentgram sends to
-- the MCP server in bearer mode. Resolution: user match > group match (by
-- position) > mcp_servers.bearer_token fallback. Mirrors agent_api_key_rules.
CREATE TABLE mcp_api_key_rules (
    id            TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    mcp_server_id TEXT NOT NULL REFERENCES mcp_servers(id) ON DELETE CASCADE,
    subject_type  TEXT NOT NULL CHECK (subject_type IN ('user', 'group')),
    subject       TEXT NOT NULL,
    api_key       TEXT NOT NULL,
    position      INT NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(mcp_server_id, subject_type, subject)
);

CREATE INDEX idx_mcp_api_key_rules_server ON mcp_api_key_rules(mcp_server_id);
