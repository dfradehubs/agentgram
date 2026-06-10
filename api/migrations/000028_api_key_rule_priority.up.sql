-- Rename the implicit position column to an explicit, admin-editable priority.
-- Lower priority is evaluated first (ORDER BY priority ASC): the lowest-numbered
-- matching group rule wins when a user belongs to several groups.

ALTER TABLE agent_api_key_rules RENAME COLUMN position TO priority;
ALTER TABLE mcp_api_key_rules RENAME COLUMN position TO priority;
