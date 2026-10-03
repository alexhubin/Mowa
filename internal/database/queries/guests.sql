-- name: CreateRoomGuest :one
INSERT INTO room_guests (id, room_id, display_name, token_hash, expires_at)
VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: GetRoomGuest :one
SELECT g.* FROM room_guests g
JOIN rooms r ON r.id = g.room_id
WHERE g.token_hash = $1 AND r.invite_code = $2 AND g.expires_at > $3 AND r.kind = 'group';

-- name: RevokeRoomGuest :exec
UPDATE room_guests SET expires_at = $3
WHERE token_hash = $1 AND room_id = $2;
