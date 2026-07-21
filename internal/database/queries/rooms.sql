-- name: CreateRoom :one
INSERT INTO rooms (id, invite_code, name, owner_id, created_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetRoomByInviteCode :one
SELECT * FROM rooms WHERE invite_code = ? LIMIT 1;

