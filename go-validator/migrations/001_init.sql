CREATE SCHEMA IF NOT EXISTS sf;

CREATE TABLE IF NOT EXISTS sf.schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sf.users (
    id            BIGSERIAL PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sf.sessions (
    token      TEXT PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES sf.users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sf.runs (
    id               BIGSERIAL PRIMARY KEY,
    user_id          BIGINT NOT NULL REFERENCES sf.users(id) ON DELETE CASCADE,
    idea_text        TEXT NOT NULL,
    canvas           JSONB,
    landing          JSONB,
    status           TEXT NOT NULL DEFAULT 'draft',
    results          JSONB,
    personas         JSONB,
    validations      JSONB,
    error_message    TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_runs_user ON sf.runs(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sf.sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sf.sessions(expires_at);
