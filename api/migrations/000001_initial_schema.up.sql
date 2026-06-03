-- Users with role
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) UNIQUE NOT NULL,
    role VARCHAR(50) NOT NULL DEFAULT 'user',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Agents
CREATE TABLE IF NOT EXISTS agents (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    category VARCHAR(100),
    protocol VARCHAR(50) NOT NULL,
    endpoint VARCHAR(500) NOT NULL,
    agent_card_path VARCHAR(255),
    forward_authorization BOOLEAN DEFAULT false,
    require_github_token BOOLEAN DEFAULT false,
    pipeline_final_agent VARCHAR(255),
    adk_app_name VARCHAR(255),
    adk_user_id VARCHAR(255),
    headers JSONB DEFAULT '{}',
    rate_limit JSONB,
    health_check JSONB,
    polling JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Agent permissions (users)
CREATE TABLE IF NOT EXISTS agent_allowed_users (
    agent_id VARCHAR(255) REFERENCES agents(id) ON DELETE CASCADE,
    user_email VARCHAR(255) NOT NULL,
    PRIMARY KEY (agent_id, user_email)
);

-- Agent permissions (groups)
CREATE TABLE IF NOT EXISTS agent_allowed_groups (
    agent_id VARCHAR(255) REFERENCES agents(id) ON DELETE CASCADE,
    group_name VARCHAR(255) NOT NULL,
    PRIMARY KEY (agent_id, group_name)
);

-- MCP servers
CREATE TABLE IF NOT EXISTS mcp_servers (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    transport VARCHAR(50) NOT NULL,
    url VARCHAR(500) NOT NULL,
    headers JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS mcp_allowed_users (
    mcp_server_id VARCHAR(255) REFERENCES mcp_servers(id) ON DELETE CASCADE,
    user_email VARCHAR(255) NOT NULL,
    PRIMARY KEY (mcp_server_id, user_email)
);

CREATE TABLE IF NOT EXISTS mcp_allowed_groups (
    mcp_server_id VARCHAR(255) REFERENCES mcp_servers(id) ON DELETE CASCADE,
    group_name VARCHAR(255) NOT NULL,
    PRIMARY KEY (mcp_server_id, group_name)
);

-- Audit log
CREATE TABLE IF NOT EXISTS audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_email VARCHAR(255) NOT NULL,
    action VARCHAR(100) NOT NULL,
    resource_type VARCHAR(50) NOT NULL,
    resource_id VARCHAR(255),
    details JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_log_created ON audit_log(created_at DESC);
