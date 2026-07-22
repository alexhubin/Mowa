-- name: EnsurePasskeyUser :one
INSERT INTO passkey_users (user_id, handle, created_at)
VALUES ($1, $2, $3)
ON CONFLICT (user_id) DO UPDATE SET user_id = EXCLUDED.user_id
RETURNING *;

-- name: GetPasskeyUser :one
SELECT * FROM passkey_users WHERE user_id = $1;

-- name: GetUserByPasskey :one
SELECT u.*
FROM passkeys p
JOIN passkey_users pu ON pu.user_id = p.user_id
JOIN users u ON u.id = p.user_id
WHERE p.credential_id = $1 AND pu.handle = $2
LIMIT 1;

-- name: ListPasskeys :many
SELECT * FROM passkeys WHERE user_id = $1 ORDER BY created_at DESC;

-- name: CountPasskeys :one
SELECT count(*) FROM passkeys WHERE user_id = $1;

-- name: CreatePasskey :one
INSERT INTO passkeys (id, user_id, credential_id, name, credential, created_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: UpdatePasskeyCredential :execrows
UPDATE passkeys
SET credential = $2, last_used_at = $3
WHERE credential_id = $1;

-- name: DeletePasskey :execrows
DELETE FROM passkeys WHERE id = $1 AND user_id = $2;

-- name: CreatePasskeyCeremony :exec
INSERT INTO passkey_ceremonies (token_hash, user_id, kind, name, session_data, expires_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ConsumePasskeyCeremony :one
DELETE FROM passkey_ceremonies
WHERE token_hash = $1 AND kind = $2 AND expires_at > $3
RETURNING *;

-- name: DeleteExpiredPasskeyCeremonies :exec
DELETE FROM passkey_ceremonies WHERE expires_at <= $1;
