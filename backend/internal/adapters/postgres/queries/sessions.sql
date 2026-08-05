-- name: CreateRefreshToken :exec
INSERT INTO refresh_tokens (id, user_id, family_id, token_hash, device_label, user_agent, ip, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: GetRefreshTokenByHash :one
SELECT * FROM refresh_tokens WHERE token_hash = $1;

-- name: GetRefreshTokenForUpdate :one
SELECT * FROM refresh_tokens WHERE id = $1 FOR UPDATE;

-- name: ReplaceRefreshToken :exec
UPDATE refresh_tokens
SET replaced_by = $2, revoked_at = $3, revoke_reason = $4, last_used_at = $3
WHERE id = $1;

-- name: RevokeRefreshTokenFamily :exec
UPDATE refresh_tokens
SET revoked_at = $2, revoke_reason = $3
WHERE family_id = $1 AND revoked_at IS NULL;

-- name: RevokeAllRefreshTokensForUser :exec
UPDATE refresh_tokens
SET revoked_at = $2, revoke_reason = $3
WHERE user_id = $1 AND revoked_at IS NULL;
