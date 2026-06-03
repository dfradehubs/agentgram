-- MCP OAuth2 support: adds auth_type, OAuth2 config, and scope mapping per group

-- New columns on mcp_servers for OAuth2 configuration
ALTER TABLE mcp_servers
    ADD COLUMN auth_type              TEXT NOT NULL DEFAULT 'none',
    ADD COLUMN oauth2_auth_server_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN oauth2_client_id       TEXT NOT NULL DEFAULT '',
    ADD COLUMN oauth2_client_secret   TEXT NOT NULL DEFAULT '',
    ADD COLUMN oauth2_scopes          TEXT NOT NULL DEFAULT '';

-- Migrate existing forward_auth servers to auth_type='forward'
UPDATE mcp_servers SET auth_type = 'forward' WHERE forward_auth = true;

-- Scope mapping: maps Agentgram groups to OAuth2 scopes per MCP server.
-- When a user belonging to group X uses MCP server Y, they get additional scopes
-- defined in this table (on top of the base scopes from mcp_servers.oauth2_scopes).
CREATE TABLE mcp_oauth2_scope_mappings (
    id             TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    mcp_server_id  TEXT NOT NULL REFERENCES mcp_servers(id) ON DELETE CASCADE,
    group_name     TEXT NOT NULL,
    scopes         TEXT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(mcp_server_id, group_name)
);

CREATE INDEX idx_mcp_oauth2_scope_mappings_server ON mcp_oauth2_scope_mappings(mcp_server_id);
