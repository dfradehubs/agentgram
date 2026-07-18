-- A disabled provider has no equivalent in v0.7.1. Preserve the safe behavior
-- by disabling every attached legacy model before removing provider metadata.
UPDATE llm_models AS m
SET enabled = false
FROM llm_providers AS p
WHERE m.provider_id = p.id AND p.enabled = false;

ALTER TABLE llm_models DROP CONSTRAINT IF EXISTS llm_models_provider_id_fkey;
DROP INDEX IF EXISTS idx_llm_models_provider_id;
ALTER TABLE llm_models DROP COLUMN IF EXISTS provider_id;
DROP TABLE IF EXISTS llm_providers;
