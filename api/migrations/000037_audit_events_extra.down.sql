DROP INDEX IF EXISTS idx_audit_events_session;
ALTER TABLE audit_events DROP COLUMN IF EXISTS llm_model;
ALTER TABLE audit_events DROP COLUMN IF EXISTS token_usage;
ALTER TABLE audit_events DROP COLUMN IF EXISTS session_id;
ALTER TABLE audit_events DROP COLUMN IF EXISTS client;
