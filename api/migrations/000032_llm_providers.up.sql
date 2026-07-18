CREATE TABLE llm_providers (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    provider_type VARCHAR(50) NOT NULL,
    api_key VARCHAR(500) NOT NULL DEFAULT '',
    endpoint VARCHAR(512) NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE llm_models ADD COLUMN provider_id VARCHAR(255);

-- Preserve every existing configuration exactly. Encrypted API keys cannot be
-- safely deduplicated because AES-GCM uses a random nonce.
INSERT INTO llm_providers (id, name, provider_type, api_key, endpoint, enabled, created_at, updated_at)
SELECT
    id,
    'Legacy: ' || name,
    CASE WHEN provider = 'openai' AND endpoint <> '' THEN 'custom' ELSE provider END,
    api_key,
    endpoint,
    true,
    created_at,
    updated_at
FROM llm_models;

UPDATE llm_models SET provider_id = id;

ALTER TABLE llm_models ALTER COLUMN provider_id SET NOT NULL;
ALTER TABLE llm_models
    ADD CONSTRAINT llm_models_provider_id_fkey
    FOREIGN KEY (provider_id) REFERENCES llm_providers(id) ON DELETE RESTRICT;

CREATE INDEX idx_llm_models_provider_id ON llm_models(provider_id);
