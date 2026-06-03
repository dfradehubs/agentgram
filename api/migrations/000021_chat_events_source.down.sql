DROP INDEX IF EXISTS idx_chat_events_source;
ALTER TABLE chat_events DROP COLUMN IF EXISTS source;
