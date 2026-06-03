-- Cannot restore data, only recreate schema
CREATE TABLE IF NOT EXISTS custom_agents (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    system_prompt TEXT DEFAULT '',
    llm_model_id TEXT NOT NULL,
    owner_email TEXT NOT NULL,
    visibility TEXT NOT NULL DEFAULT 'private',
    sub_agent_ids JSONB DEFAULT '[]',
    mcp_server_ids JSONB DEFAULT '[]',
    shared_users JSONB DEFAULT '[]',
    shared_groups JSONB DEFAULT '[]',
    admin_users JSONB DEFAULT '[]',
    admin_groups JSONB DEFAULT '[]',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_custom_agents_owner ON custom_agents(owner_email);
CREATE INDEX IF NOT EXISTS idx_custom_agents_visibility ON custom_agents(visibility);
