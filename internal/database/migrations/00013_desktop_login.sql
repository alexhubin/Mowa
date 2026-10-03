-- +goose Up
CREATE TABLE desktop_logins (
 id TEXT PRIMARY KEY,
 challenge TEXT NOT NULL,
 user_id TEXT REFERENCES users(id) ON DELETE CASCADE,
 expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX desktop_logins_expires_idx ON desktop_logins(expires_at);

-- +goose Down
DROP TABLE desktop_logins;
