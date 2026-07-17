-- Per-group cap on moderated-debate turns. 0 = use the surface default
-- (6 for the SSE endpoint, 3 for the synchronous MCP tool).
ALTER TABLE agent_groups ADD COLUMN IF NOT EXISTS max_turns INTEGER NOT NULL DEFAULT 0;
