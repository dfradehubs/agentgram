-- Optional custom API endpoint for LLM models (OpenAI-compatible gateways,
-- Ollama, LiteLLM, local mocks). Empty = provider's default endpoint.
ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS endpoint VARCHAR(512) NOT NULL DEFAULT '';
