-- name: GrantConsent :one
INSERT INTO privacy_consents (user_id, purpose, policy_version, source)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: FindActiveConsent :one
SELECT * FROM privacy_consents WHERE user_id = $1 AND purpose = $2 AND withdrawn_at IS NULL;

-- name: WithdrawActiveConsent :exec
UPDATE privacy_consents SET withdrawn_at = $3 WHERE user_id = $1 AND purpose = $2 AND withdrawn_at IS NULL;
