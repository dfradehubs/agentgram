-- Audit events: detailed per-interaction records including the prompt sent and
-- the response returned (truncated). Distinct from chat_events, which stores
-- only aggregate metrics with no content.
CREATE TABLE audit_events (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_email    VARCHAR(255) NOT NULL,
    user_groups   TEXT[],
    resource_type VARCHAR(20)  NOT NULL,   -- agent, mcp, skill, group
    resource_id   VARCHAR(255) NOT NULL,
    resource_name VARCHAR(255),
    source        VARCHAR(20)  NOT NULL DEFAULT 'web', -- web, mcp, slack
    action        VARCHAR(30)  NOT NULL,   -- chat, mcp_tool, skill_read, group_debate
    prompt        TEXT,
    response      TEXT,
    tool_calls    JSONB,
    status        VARCHAR(20)  NOT NULL,   -- ok, error
    error_type    VARCHAR(50),
    error_msg     TEXT,
    duration_ms   INTEGER      NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_events_created  ON audit_events (created_at DESC);
CREATE INDEX idx_audit_events_user     ON audit_events (user_email, created_at DESC);
CREATE INDEX idx_audit_events_resource ON audit_events (resource_type, created_at DESC);
