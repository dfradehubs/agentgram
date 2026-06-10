DROP TABLE IF EXISTS agent_api_key_rules;

ALTER TABLE agents
    DROP COLUMN IF EXISTS auth_type,
    DROP COLUMN IF EXISTS bearer_token,
    DROP COLUMN IF EXISTS auth_header_name;
