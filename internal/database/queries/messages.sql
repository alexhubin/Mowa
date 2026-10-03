-- name: CreateRoomMessage :one
INSERT INTO room_messages (id, room_id, user_id, body, created_at, guest_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListRoomMessages :many
SELECT * FROM (
    SELECT
        m.id,
        m.room_id,
        COALESCE(m.user_id, m.guest_id)::text AS author_id,
        m.body,
        m.created_at,
        COALESCE(u.username, '')::text AS username,
        COALESCE(u.display_name, g.display_name)::text AS display_name
    FROM room_messages m
    LEFT JOIN users u ON u.id = m.user_id
    LEFT JOIN room_guests g ON g.id = m.guest_id
    WHERE m.room_id = $1
    ORDER BY m.created_at DESC, m.id DESC
    LIMIT 100
) recent
ORDER BY recent.created_at, recent.id;
