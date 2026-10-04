-- +goose Up
CREATE TABLE desktop_diagnostics (
 user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 event_id TEXT NOT NULL,
 call_id TEXT NOT NULL,
 received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 payload JSONB NOT NULL,
 PRIMARY KEY (user_id, event_id)
);
CREATE INDEX desktop_diagnostics_user_time ON desktop_diagnostics(user_id, received_at);
CREATE INDEX desktop_diagnostics_time ON desktop_diagnostics(received_at);
-- +goose Down
DROP TABLE desktop_diagnostics;
