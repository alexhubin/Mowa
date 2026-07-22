-- +goose Up
CREATE TABLE passkey_users (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    handle BYTEA NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE passkeys (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES passkey_users(user_id) ON DELETE CASCADE,
    credential_id BYTEA NOT NULL UNIQUE,
    name VARCHAR(50) NOT NULL,
    credential JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ
);
CREATE INDEX passkeys_user_id_idx ON passkeys(user_id);

CREATE TABLE passkey_ceremonies (
    token_hash CHAR(64) PRIMARY KEY,
    user_id TEXT REFERENCES users(id) ON DELETE CASCADE,
    kind VARCHAR(16) NOT NULL,
    name VARCHAR(50) NOT NULL DEFAULT '',
    session_data JSONB NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT passkey_ceremonies_kind_check CHECK (kind IN ('registration', 'login'))
);
CREATE INDEX passkey_ceremonies_expires_at_idx ON passkey_ceremonies(expires_at);

-- +goose Down
DROP TABLE passkey_ceremonies;
DROP TABLE passkeys;
DROP TABLE passkey_users;
