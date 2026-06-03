ALTER TABLE mcp_servers DROP COLUMN IF EXISTS allowed_groups;
ALTER TABLE mcp_servers DROP COLUMN IF EXISTS allowed_users;
ALTER TABLE users DROP COLUMN IF EXISTS last_access_at;
