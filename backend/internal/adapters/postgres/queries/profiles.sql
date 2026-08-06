-- name: UpsertProfile :exec
INSERT INTO profiles (user_id, bio, interests, city, location, questionnaire, onboarding_completed)
VALUES (
    sqlc.arg(user_id), sqlc.narg(bio), sqlc.arg(interests), sqlc.narg(city),
    CASE
        WHEN sqlc.narg(longitude)::float8 IS NULL OR sqlc.narg(latitude)::float8 IS NULL THEN NULL
        ELSE ST_SetSRID(ST_MakePoint(sqlc.narg(longitude)::float8, sqlc.narg(latitude)::float8), 4326)::geography
    END,
    sqlc.arg(questionnaire), sqlc.arg(onboarding_completed)
)
ON CONFLICT (user_id) DO UPDATE SET
    bio = EXCLUDED.bio,
    interests = EXCLUDED.interests,
    city = EXCLUDED.city,
    -- An update that carries no coordinates must preserve the stored ones;
    -- only an explicit clear_location drops them.
    location = CASE
        WHEN sqlc.arg(clear_location)::boolean THEN NULL
        WHEN EXCLUDED.location IS NULL THEN profiles.location
        ELSE EXCLUDED.location
    END,
    questionnaire = EXCLUDED.questionnaire,
    onboarding_completed = EXCLUDED.onboarding_completed,
    updated_at = now();

-- name: GetProfile :one
SELECT
    user_id, bio, interests, city, questionnaire, onboarding_completed, created_at, updated_at,
    (location IS NOT NULL) AS has_location
FROM profiles
WHERE user_id = $1;

-- name: UpsertPreferences :exec
INSERT INTO user_preferences (user_id, min_age, max_age, max_distance_km)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id) DO UPDATE SET
    min_age = EXCLUDED.min_age,
    max_age = EXCLUDED.max_age,
    max_distance_km = EXCLUDED.max_distance_km,
    updated_at = now();

-- name: GetPreferences :one
SELECT * FROM user_preferences WHERE user_id = $1;

-- name: UpdateGenders :exec
UPDATE user_preferences SET genders = $2, updated_at = now() WHERE user_id = $1;

-- name: ClearGenders :exec
UPDATE user_preferences SET genders = '{}', updated_at = now() WHERE user_id = $1;

-- name: CreatePhoto :one
INSERT INTO photos (id, user_id, storage_key, mime_type, byte_size, width, height, position, is_primary)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: ListPhotos :many
SELECT * FROM photos WHERE user_id = $1 AND deleted_at IS NULL ORDER BY position;

-- name: GetPhoto :one
SELECT * FROM photos WHERE id = $1 AND deleted_at IS NULL;

-- name: CountActivePhotos :one
SELECT count(*) FROM photos WHERE user_id = $1 AND deleted_at IS NULL;

-- name: SetPhotoPosition :exec
UPDATE photos SET position = $3 WHERE id = $1 AND user_id = $2;

-- name: ClearPrimaryPhoto :exec
UPDATE photos SET is_primary = false WHERE user_id = $1 AND is_primary AND deleted_at IS NULL;

-- name: SetPrimaryPhoto :exec
UPDATE photos SET is_primary = true WHERE id = $1 AND user_id = $2;

-- name: SoftDeletePhoto :exec
UPDATE photos SET deleted_at = $2 WHERE id = $1;
