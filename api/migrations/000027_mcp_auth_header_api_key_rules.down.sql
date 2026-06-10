DROP TABLE IF EXISTS mcp_api_key_rules;

ALTER TABLE mcp_servers
    DROP COLUMN IF EXISTS auth_header_name;
