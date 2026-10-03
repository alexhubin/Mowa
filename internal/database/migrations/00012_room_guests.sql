-- +goose Up
CREATE TABLE room_guests (
    id TEXT PRIMARY KEY,
    room_id TEXT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    display_name VARCHAR(60) NOT NULL CHECK (char_length(display_name) BETWEEN 1 AND 60),
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    UNIQUE (id, room_id)
);
CREATE INDEX room_guests_room_idx ON room_guests(room_id);
ALTER TABLE room_messages ALTER COLUMN user_id DROP NOT NULL;
ALTER TABLE room_messages ADD COLUMN guest_id TEXT;
ALTER TABLE room_messages ADD CONSTRAINT room_messages_guest_fk
    FOREIGN KEY (guest_id, room_id) REFERENCES room_guests(id, room_id) ON DELETE CASCADE;
ALTER TABLE room_messages ADD CONSTRAINT room_messages_author_check
    CHECK ((user_id IS NOT NULL) <> (guest_id IS NOT NULL));

-- +goose Down
DELETE FROM room_messages WHERE guest_id IS NOT NULL;
ALTER TABLE room_messages DROP CONSTRAINT room_messages_author_check;
ALTER TABLE room_messages DROP CONSTRAINT room_messages_guest_fk;
ALTER TABLE room_messages DROP COLUMN guest_id;
ALTER TABLE room_messages ALTER COLUMN user_id SET NOT NULL;
DROP TABLE room_guests;
