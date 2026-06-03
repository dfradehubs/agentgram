CREATE TABLE custom_agents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    system_prompt TEXT NOT NULL,
    llm_model_id VARCHAR(255) NOT NULL REFERENCES llm_models(id),
    owner_email VARCHAR(255) NOT NULL,
    visibility VARCHAR(20) NOT NULL DEFAULT 'private',
    sub_agent_ids JSONB DEFAULT '[]',
    mcp_server_ids JSONB DEFAULT '[]',
    shared_users JSONB DEFAULT '[]',
    shared_groups JSONB DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_custom_agents_owner ON custom_agents(owner_email);
CREATE INDEX idx_custom_agents_visibility ON custom_agents(visibility);
