CREATE TABLE chat_events (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    resource_type   VARCHAR(20)  NOT NULL,
    resource_id     VARCHAR(255) NOT NULL,
    resource_name   VARCHAR(255),
    protocol        VARCHAR(50),
    user_email      VARCHAR(255) NOT NULL,
    session_id      VARCHAR(255),
    status          VARCHAR(20)  NOT NULL,
    error_type      VARCHAR(50),
    error_msg       TEXT,
    duration_ms     INTEGER      NOT NULL,
    ttfb_ms         INTEGER,
    message_count   INTEGER      NOT NULL,
    tool_calls      JSONB,
    token_usage     JSONB,
    llm_model       VARCHAR(255),
    session_rotated BOOLEAN DEFAULT FALSE,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_chat_events_resource ON chat_events (resource_type, resource_id, created_at DESC);
CREATE INDEX idx_chat_events_user     ON chat_events (user_email, created_at DESC);
CREATE INDEX idx_chat_events_created  ON chat_events (created_at DESC);
CREATE INDEX idx_chat_events_status   ON chat_events (resource_id, status, created_at DESC);
