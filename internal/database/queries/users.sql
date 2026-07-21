-- name: CreateUser :one
INSERT INTO users (id, email, display_name, password_hash, created_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = ? COLLATE NOCASE LIMIT 1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = ? LIMIT 1;

