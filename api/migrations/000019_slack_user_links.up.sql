CREATE TABLE slack_user_links (
    slack_user_id   TEXT PRIMARY KEY,
    email           TEXT NOT NULL,
    refresh_token   TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_slack_user_links_email ON slack_user_links(email);
