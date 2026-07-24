-- Richer audit event fields: MCP client/origin, session correlation, token usage
-- and LLM model. tool_calls (already JSONB) now stores name+arguments+result.
ALTER TABLE audit_events ADD COLUMN IF NOT EXISTS client      VARCHAR(255);
ALTER TABLE audit_events ADD COLUMN IF NOT EXISTS session_id  VARCHAR(255);
ALTER TABLE audit_events ADD COLUMN IF NOT EXISTS token_usage JSONB;
ALTER TABLE audit_events ADD COLUMN IF NOT EXISTS llm_model   VARCHAR(255);

CREATE INDEX IF NOT EXISTS idx_audit_events_session ON audit_events (session_id);
