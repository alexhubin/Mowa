-- name: CreateUser :one
INSERT INTO users (id, username, email, display_name, password_hash, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $6)
RETURNING *;

-- name: CreateUserSettings :one
INSERT INTO user_settings (user_id, video_quality, updated_at)
VALUES ($1, 'high', $2)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE lower(email) = lower($1) LIMIT 1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1 LIMIT 1;

-- name: GetUserByUsername :one
SELECT * FROM users WHERE lower(username) = lower($1) LIMIT 1;

-- name: SearchUsers :many
SELECT
    u.id,
    u.username,
    u.display_name,
    CASE
        WHEN EXISTS (
            SELECT 1 FROM friendships f
            WHERE (f.user_id = $1 AND f.friend_id = u.id)
               OR (f.user_id = u.id AND f.friend_id = $1)
        ) THEN 'friends'
        WHEN EXISTS (
            SELECT 1 FROM friend_requests fr
            WHERE fr.sender_id = $1 AND fr.receiver_id = u.id
        ) THEN 'request_sent'
        WHEN EXISTS (
            SELECT 1 FROM friend_requests fr
            WHERE fr.sender_id = u.id AND fr.receiver_id = $1
        ) THEN 'request_received'
        ELSE 'none'
    END AS relationship
FROM users u
WHERE u.id <> $1
  AND (lower(u.username) LIKE '%' || lower($2) || '%' OR lower(u.display_name) LIKE '%' || lower($2) || '%')
ORDER BY CASE WHEN lower(u.username) = lower($2) THEN 0 ELSE 1 END, lower(u.username)
LIMIT 20;

-- name: UpdateProfile :one
UPDATE users
SET username = $2, display_name = $3, updated_at = $4
WHERE id = $1
RETURNING *;

-- name: UpdatePassword :exec
UPDATE users
SET password_hash = $2, must_change_password = FALSE, updated_at = $3
WHERE id = $1;

-- name: GetUserSettings :one
SELECT * FROM user_settings WHERE user_id = $1;

-- name: UpdateUserSettings :one
INSERT INTO user_settings (user_id, video_quality, updated_at)
VALUES ($1, $2, $3)
ON CONFLICT (user_id) DO UPDATE
SET video_quality = EXCLUDED.video_quality, updated_at = EXCLUDED.updated_at
RETURNING *;
