DROP TABLE IF EXISTS mcp_oauth2_scope_mappings;

ALTER TABLE mcp_servers
    DROP COLUMN IF EXISTS auth_type,
    DROP COLUMN IF EXISTS oauth2_auth_server_url,
    DROP COLUMN IF EXISTS oauth2_client_id,
    DROP COLUMN IF EXISTS oauth2_client_secret,
    DROP COLUMN IF EXISTS oauth2_scopes;
