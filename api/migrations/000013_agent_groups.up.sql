CREATE TABLE agent_groups (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    agent_ids JSONB NOT NULL DEFAULT '[]',
    created_by VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE agent_group_allowed_users (
    group_id VARCHAR(255) REFERENCES agent_groups(id) ON DELETE CASCADE,
    user_email VARCHAR(255) NOT NULL,
    PRIMARY KEY (group_id, user_email)
);

CREATE TABLE agent_group_allowed_groups (
    group_id VARCHAR(255) REFERENCES agent_groups(id) ON DELETE CASCADE,
    group_name VARCHAR(255) NOT NULL,
    PRIMARY KEY (group_id, group_name)
);
