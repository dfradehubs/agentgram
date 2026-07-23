-- Optional per-model output token cap. 0 = auto: each caller uses its built-in
-- per-role default (e.g. the moderator needs headroom for Claude thinking).
ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS max_tokens INTEGER NOT NULL DEFAULT 0;
