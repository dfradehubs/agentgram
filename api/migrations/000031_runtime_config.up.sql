-- Runtime-editable operational settings (General Configuration in the admin
-- panel). Separate from app_settings (which holds encryption keys). Key/value;
-- unknown keys are ignored, missing keys fall back to the code default. Infra/
-- deploy config stays in YAML.
CREATE TABLE IF NOT EXISTS runtime_config (
    key        VARCHAR(128) PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
