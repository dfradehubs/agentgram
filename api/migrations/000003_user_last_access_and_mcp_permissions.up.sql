-- Add last_access_at to users table
ALTER TABLE users ADD COLUMN IF NOT EXISTS last_access_at TIMESTAMPTZ;

-- Add allowed_users and allowed_groups JSONB columns to mcp_servers
-- (used by Update handler alongside the permission junction tables)
ALTER TABLE mcp_servers ADD COLUMN IF NOT EXISTS allowed_users JSONB DEFAULT '[]';
ALTER TABLE mcp_servers ADD COLUMN IF NOT EXISTS allowed_groups JSONB DEFAULT '[]';
