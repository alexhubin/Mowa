-- name: CreateRoomMessage :one
INSERT INTO room_messages (id, room_id, user_id, body, created_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListRoomMessages :many
SELECT
    m.id,
    m.room_id,
    m.user_id,
    m.body,
    m.created_at,
    u.username,
    u.display_name
FROM room_messages m
JOIN users u ON u.id = m.user_id
WHERE m.room_id = $1
ORDER BY m.created_at DESC, m.id DESC
LIMIT 100;
