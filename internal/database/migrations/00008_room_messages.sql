-- +goose Up
CREATE TABLE room_messages (
    id TEXT PRIMARY KEY,
    room_id TEXT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    body VARCHAR(2000) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT room_messages_body_length CHECK (char_length(body) BETWEEN 1 AND 2000)
);
CREATE INDEX room_messages_room_created_idx ON room_messages(room_id, created_at DESC, id DESC);

-- +goose Down
DROP TABLE room_messages;
