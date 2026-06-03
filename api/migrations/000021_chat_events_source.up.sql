ALTER TABLE chat_events ADD COLUMN source VARCHAR(20) NOT NULL DEFAULT 'web';
CREATE INDEX idx_chat_events_source ON chat_events(source);
