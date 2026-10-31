CREATE TABLE sessions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash   BYTEA       NOT NULL UNIQUE, -- SHA-256 of the opaque token, never the token itself
    user_agent   TEXT        NOT NULL DEFAULT '',
    ip_address   INET,
    expires_at   TIMESTAMPTZ NOT NULL,
    revoked_at   TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_sessions_user_id            ON sessions (user_id);
CREATE INDEX idx_sessions_expires_at         ON sessions (expires_at);
CREATE INDEX idx_sessions_user_id_revoked_at ON sessions (user_id, revoked_at);
