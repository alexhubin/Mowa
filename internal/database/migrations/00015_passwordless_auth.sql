-- +goose Up
CREATE TABLE auth_flows (
 token_hash TEXT PRIMARY KEY,
 kind TEXT NOT NULL CHECK (kind IN ('otp','google','identity')),
 email TEXT NOT NULL DEFAULT '',
 subject TEXT NOT NULL DEFAULT '',
 code_hash TEXT NOT NULL DEFAULT '',
 attempts INTEGER NOT NULL DEFAULT 0,
 next_path TEXT NOT NULL DEFAULT '/',
 nonce TEXT NOT NULL DEFAULT '',
 verifier TEXT NOT NULL DEFAULT '',
 expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX auth_flows_expiry_idx ON auth_flows(expires_at);
CREATE TABLE auth_rate_limits (
 key_hash TEXT PRIMARY KEY,
 count INTEGER NOT NULL,
 expires_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE google_identities (
 subject TEXT PRIMARY KEY,
 user_id TEXT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE
);
-- +goose Down
DROP TABLE google_identities;
DROP TABLE auth_rate_limits;
DROP TABLE auth_flows;
