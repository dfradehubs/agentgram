CREATE TABLE agent_group_sessions (
    group_id VARCHAR(255) REFERENCES agent_groups(id) ON DELETE CASCADE,
    session_id VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (group_id, session_id)
);
