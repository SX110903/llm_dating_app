-- name: CreateUser :one
INSERT INTO users (id, email, password_hash, display_name, birth_date, gender, password_changed_at)
VALUES ($1, $2, $3, $4, $5, $6, now())
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: UpdateUserPasswordHash :exec
UPDATE users
SET password_hash = $2, password_changed_at = $3, updated_at = now()
WHERE id = $1;
