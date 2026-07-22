-- +goose Up
ALTER TABLE users
ADD COLUMN must_change_password BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE users
ALTER COLUMN must_change_password SET DEFAULT TRUE;

-- +goose Down
ALTER TABLE users
DROP COLUMN must_change_password;
