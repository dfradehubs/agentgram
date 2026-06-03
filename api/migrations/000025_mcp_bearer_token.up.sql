-- MCP bearer token: adds static bearer token support for MCP servers
ALTER TABLE mcp_servers
    ADD COLUMN bearer_token TEXT NOT NULL DEFAULT '';
