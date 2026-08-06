-- name: EnsureDiscoveryReady :one
SELECT EXISTS (
    SELECT 1
    FROM users u
    JOIN profiles p ON p.user_id = u.id
    JOIN user_preferences pref ON pref.user_id = u.id
    JOIN privacy_consents consent
      ON consent.user_id = u.id
     AND consent.purpose = 'matching_gender_preferences'
     AND consent.withdrawn_at IS NULL
    WHERE u.id = sqlc.arg(user_id)
      AND u.status = 'active'
      AND p.onboarding_completed
      AND p.location IS NOT NULL
      AND cardinality(pref.genders) > 0
      AND EXISTS (
          SELECT 1 FROM photos photo
          WHERE photo.user_id = u.id AND photo.is_primary AND photo.deleted_at IS NULL
      )
) AS ready;

-- name: ListDiscoveryCandidates :many
WITH viewer AS (
    SELECT
        u.id,
        u.gender,
        u.birth_date,
        p.location,
        p.interests,
        p.questionnaire,
        pref.min_age,
        pref.max_age,
        pref.max_distance_km,
        pref.genders
    FROM users u
    JOIN profiles p ON p.user_id = u.id
    JOIN user_preferences pref ON pref.user_id = u.id
    JOIN privacy_consents consent
      ON consent.user_id = u.id
     AND consent.purpose = 'matching_gender_preferences'
     AND consent.withdrawn_at IS NULL
    WHERE u.id = sqlc.arg(viewer_id)
      AND u.status = 'active'
      AND p.onboarding_completed
      AND p.location IS NOT NULL
      AND cardinality(pref.genders) > 0
),
eligible AS (
    SELECT
        candidate.id AS user_id,
        candidate.display_name,
        EXTRACT(YEAR FROM age(sqlc.arg(as_of)::timestamptz, candidate.birth_date))::integer AS age,
        candidate.gender,
        candidate.last_active_at,
        candidate_profile.bio,
        candidate_profile.interests,
        candidate_profile.city,
        candidate_profile.questionnaire,
        photo.id AS primary_photo_id,
        ST_Distance(v.location, candidate_profile.location)::float8 / 1000.0 AS distance_km,
        LEAST(v.max_distance_km, candidate_pref.max_distance_km)::float8 AS effective_distance_km,
        v.interests AS viewer_interests,
        v.questionnaire AS viewer_questionnaire
    FROM viewer v
    JOIN users candidate ON candidate.id <> v.id AND candidate.status = 'active'
    JOIN profiles candidate_profile
      ON candidate_profile.user_id = candidate.id
     AND candidate_profile.onboarding_completed
     AND candidate_profile.location IS NOT NULL
    JOIN user_preferences candidate_pref ON candidate_pref.user_id = candidate.id
    JOIN privacy_consents candidate_consent
      ON candidate_consent.user_id = candidate.id
     AND candidate_consent.purpose = 'matching_gender_preferences'
     AND candidate_consent.withdrawn_at IS NULL
    JOIN photos photo
      ON photo.user_id = candidate.id
     AND photo.is_primary
     AND photo.deleted_at IS NULL
    WHERE cardinality(candidate_pref.genders) > 0
      AND candidate.gender = ANY(v.genders)
      AND v.gender = ANY(candidate_pref.genders)
      AND EXTRACT(YEAR FROM age(sqlc.arg(as_of)::timestamptz, candidate.birth_date))::integer
          BETWEEN v.min_age AND v.max_age
      AND EXTRACT(YEAR FROM age(sqlc.arg(as_of)::timestamptz, v.birth_date))::integer
          BETWEEN candidate_pref.min_age AND candidate_pref.max_age
      AND ST_DWithin(
          v.location,
          candidate_profile.location,
          LEAST(v.max_distance_km, candidate_pref.max_distance_km)::float8 * 1000.0
      )
      AND NOT EXISTS (
          SELECT 1 FROM swipes swipe
          WHERE swipe.actor_id = v.id AND swipe.target_id = candidate.id
      )
      AND NOT EXISTS (
          SELECT 1 FROM blocks block
          WHERE (block.blocker_id = v.id AND block.blocked_id = candidate.id)
             OR (block.blocker_id = candidate.id AND block.blocked_id = v.id)
      )
      AND NOT EXISTS (
          SELECT 1 FROM matches match
          WHERE match.user_low_id = LEAST(v.id, candidate.id)
            AND match.user_high_id = GREATEST(v.id, candidate.id)
      )
),
components AS (
    SELECT
        eligible.*,
        COALESCE(
            (
                SELECT count(*)::float8
                FROM (
                    SELECT lower(trim(value)) AS item
                    FROM unnest(eligible.viewer_interests) AS value
                    WHERE trim(value) <> ''
                    INTERSECT
                    SELECT lower(trim(value)) AS item
                    FROM unnest(eligible.interests) AS value
                    WHERE trim(value) <> ''
                ) shared
            ) / NULLIF(
                (
                    SELECT count(*)::float8
                    FROM (
                        SELECT lower(trim(value)) AS item
                        FROM unnest(eligible.viewer_interests) AS value
                        WHERE trim(value) <> ''
                        UNION
                        SELECT lower(trim(value)) AS item
                        FROM unnest(eligible.interests) AS value
                        WHERE trim(value) <> ''
                    ) combined
                ),
                0
            ),
            0
        )::float8 AS interests_score,
        COALESCE(
            (
                SELECT count(*)::float8
                FROM jsonb_each(eligible.viewer_questionnaire) viewer_answer
                JOIN jsonb_each(eligible.questionnaire) candidate_answer USING (key)
                WHERE viewer_answer.value = candidate_answer.value
            ) / NULLIF(
                (
                    SELECT count(*)::float8
                    FROM (
                        SELECT jsonb_object_keys(eligible.viewer_questionnaire) AS key
                        UNION
                        SELECT jsonb_object_keys(eligible.questionnaire) AS key
                    ) questionnaire_keys
                ),
                0
            ),
            0
        )::float8 AS questionnaire_score,
        GREATEST(0, LEAST(1, 1 - eligible.distance_km / NULLIF(eligible.effective_distance_km, 0)))::float8
            AS distance_score,
        GREATEST(
            0,
            LEAST(
                1,
                1 - EXTRACT(EPOCH FROM (sqlc.arg(as_of)::timestamptz - eligible.last_active_at))
                    / (sqlc.arg(activity_window_hours)::float8 * 3600.0)
            )
        )::float8 AS activity_score
    FROM eligible
),
ranked AS (
    SELECT
        components.*,
        (
            components.interests_score * sqlc.arg(interests_weight)::float8
          + components.questionnaire_score * sqlc.arg(questionnaire_weight)::float8
          + components.distance_score * sqlc.arg(distance_weight)::float8
          + components.activity_score * sqlc.arg(activity_weight)::float8
        ) / NULLIF(
            sqlc.arg(interests_weight)::float8
          + sqlc.arg(questionnaire_weight)::float8
          + sqlc.arg(distance_weight)::float8
          + sqlc.arg(activity_weight)::float8,
            0
        ) AS total_score
    FROM components
)
SELECT
    user_id,
    display_name,
    age,
    gender,
    bio,
    interests,
    city,
    distance_km::float8 AS distance_km,
    last_active_at,
    primary_photo_id,
    interests_score,
    questionnaire_score,
    distance_score,
    activity_score,
    total_score::float8
FROM ranked
WHERE sqlc.narg(cursor_score)::float8 IS NULL
   OR total_score < sqlc.narg(cursor_score)::float8
   OR (total_score = sqlc.narg(cursor_score)::float8 AND user_id > sqlc.narg(cursor_user_id)::uuid)
ORDER BY total_score DESC, user_id
LIMIT sqlc.arg(page_limit);

-- name: CanSwipeTarget :one
WITH viewer AS (
    SELECT u.id, u.gender, u.birth_date, p.location, pref.min_age, pref.max_age, pref.max_distance_km, pref.genders
    FROM users u
    JOIN profiles p ON p.user_id = u.id
    JOIN user_preferences pref ON pref.user_id = u.id
    JOIN privacy_consents consent
      ON consent.user_id = u.id
     AND consent.purpose = 'matching_gender_preferences'
     AND consent.withdrawn_at IS NULL
    WHERE u.id = sqlc.arg(actor_id)
      AND u.status = 'active'
      AND p.onboarding_completed
      AND p.location IS NOT NULL
      AND cardinality(pref.genders) > 0
)
SELECT EXISTS (
    SELECT 1
    FROM viewer v
    JOIN users candidate ON candidate.id = sqlc.arg(target_id) AND candidate.status = 'active'
    JOIN profiles candidate_profile
      ON candidate_profile.user_id = candidate.id
     AND candidate_profile.onboarding_completed
     AND candidate_profile.location IS NOT NULL
    JOIN user_preferences candidate_pref ON candidate_pref.user_id = candidate.id
    JOIN privacy_consents candidate_consent
      ON candidate_consent.user_id = candidate.id
     AND candidate_consent.purpose = 'matching_gender_preferences'
     AND candidate_consent.withdrawn_at IS NULL
    WHERE cardinality(candidate_pref.genders) > 0
      AND candidate.gender = ANY(v.genders)
      AND v.gender = ANY(candidate_pref.genders)
      AND EXTRACT(YEAR FROM age(sqlc.arg(as_of)::timestamptz, candidate.birth_date))::integer
          BETWEEN v.min_age AND v.max_age
      AND EXTRACT(YEAR FROM age(sqlc.arg(as_of)::timestamptz, v.birth_date))::integer
          BETWEEN candidate_pref.min_age AND candidate_pref.max_age
      AND ST_DWithin(
          v.location,
          candidate_profile.location,
          LEAST(v.max_distance_km, candidate_pref.max_distance_km)::float8 * 1000.0
      )
      AND EXISTS (
          SELECT 1 FROM photos photo
          WHERE photo.user_id = candidate.id AND photo.is_primary AND photo.deleted_at IS NULL
      )
      AND NOT EXISTS (
          SELECT 1 FROM matches match
          WHERE match.user_low_id = LEAST(v.id, candidate.id)
            AND match.user_high_id = GREATEST(v.id, candidate.id)
      )
) AS allowed;

-- name: LockInteractionPair :exec
SELECT pg_advisory_xact_lock(
    hashtextextended(
        LEAST(sqlc.arg(first_user_id)::uuid, sqlc.arg(second_user_id)::uuid)::text
        || ':' ||
        GREATEST(sqlc.arg(first_user_id)::uuid, sqlc.arg(second_user_id)::uuid)::text,
        0
    )
);

-- name: InteractionBlocked :one
SELECT EXISTS (
    SELECT 1 FROM blocks
    WHERE (blocker_id = sqlc.arg(first_user_id) AND blocked_id = sqlc.arg(second_user_id))
       OR (blocker_id = sqlc.arg(second_user_id) AND blocked_id = sqlc.arg(first_user_id))
) AS blocked;

-- name: GetSwipe :one
SELECT * FROM swipes WHERE actor_id = $1 AND target_id = $2;

-- name: InsertSwipe :one
INSERT INTO swipes (id, actor_id, target_id, action, created_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (actor_id, target_id) DO NOTHING
RETURNING *;

-- name: HasPositiveReverseSwipe :one
SELECT EXISTS (
    SELECT 1 FROM swipes
    WHERE actor_id = sqlc.arg(target_id)
      AND target_id = sqlc.arg(actor_id)
      AND action IN ('like', 'superlike')
) AS positive;

-- name: InsertMatch :one
INSERT INTO matches (id, user_low_id, user_high_id, matched_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_low_id, user_high_id) DO UPDATE
SET user_low_id = EXCLUDED.user_low_id
RETURNING *;

-- name: GetMatchByPair :one
SELECT * FROM matches WHERE user_low_id = $1 AND user_high_id = $2;

-- name: CountSwipesSince :one
SELECT count(*) FROM swipes WHERE actor_id = $1 AND created_at >= $2;

-- name: ListActiveMatches :many
SELECT
    match.*,
    other.id AS other_user_id,
    other.display_name,
    other_profile.bio,
    other_profile.city,
    other.last_active_at AS other_last_active_at,
    photo.id AS primary_photo_id
FROM matches match
JOIN users other ON other.id = CASE
    WHEN match.user_low_id = sqlc.arg(user_id) THEN match.user_high_id
    ELSE match.user_low_id
END
JOIN profiles other_profile ON other_profile.user_id = other.id
JOIN photos photo ON photo.user_id = other.id AND photo.is_primary AND photo.deleted_at IS NULL
WHERE (match.user_low_id = sqlc.arg(user_id) OR match.user_high_id = sqlc.arg(user_id))
  AND match.unmatched_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM blocks block
      WHERE (block.blocker_id = sqlc.arg(user_id) AND block.blocked_id = other.id)
         OR (block.blocker_id = other.id AND block.blocked_id = sqlc.arg(user_id))
  )
  AND (
      sqlc.narg(cursor_matched_at)::timestamptz IS NULL
      OR match.matched_at < sqlc.narg(cursor_matched_at)::timestamptz
      OR (
          match.matched_at = sqlc.narg(cursor_matched_at)::timestamptz
          AND match.id < sqlc.narg(cursor_match_id)::uuid
      )
  )
ORDER BY match.matched_at DESC, match.id DESC
LIMIT sqlc.arg(page_limit);

-- name: Unmatch :execrows
UPDATE matches
SET unmatched_at = sqlc.arg(unmatched_at), unmatched_by = sqlc.arg(actor_id)
WHERE id = sqlc.arg(match_id)
  AND (user_low_id = sqlc.arg(actor_id) OR user_high_id = sqlc.arg(actor_id))
  AND unmatched_at IS NULL;

-- name: MatchParticipantExists :one
SELECT EXISTS (
    SELECT 1 FROM matches
    WHERE id = sqlc.arg(match_id)
      AND (user_low_id = sqlc.arg(actor_id) OR user_high_id = sqlc.arg(actor_id))
) AS exists;

-- name: InsertBlock :exec
INSERT INTO blocks (blocker_id, blocked_id, created_at)
VALUES ($1, $2, $3)
ON CONFLICT (blocker_id, blocked_id) DO NOTHING;

-- name: UnmatchPairForBlock :exec
UPDATE matches
SET unmatched_at = sqlc.arg(unmatched_at), unmatched_by = sqlc.arg(blocker_id)
WHERE user_low_id = LEAST(sqlc.arg(blocker_id)::uuid, sqlc.arg(blocked_id)::uuid)
  AND user_high_id = GREATEST(sqlc.arg(blocker_id)::uuid, sqlc.arg(blocked_id)::uuid)
  AND unmatched_at IS NULL;

-- name: InsertReport :one
INSERT INTO reports (id, reporter_id, reported_id, reason, description, status, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetVisibleMatchingPhoto :one
SELECT photo.id, photo.user_id, photo.storage_key, photo.mime_type
FROM photos photo
WHERE photo.id = sqlc.arg(photo_id)
  AND photo.is_primary
  AND photo.deleted_at IS NULL
  AND (
      photo.user_id = sqlc.arg(viewer_id)
      OR EXISTS (
          SELECT 1 FROM matches match
          WHERE match.user_low_id = LEAST(sqlc.arg(viewer_id)::uuid, photo.user_id)
            AND match.user_high_id = GREATEST(sqlc.arg(viewer_id)::uuid, photo.user_id)
            AND match.unmatched_at IS NULL
      )
      OR EXISTS (
          SELECT 1
          FROM users viewer_user
          JOIN profiles viewer_profile ON viewer_profile.user_id = viewer_user.id
          JOIN user_preferences viewer_pref ON viewer_pref.user_id = viewer_user.id
          JOIN privacy_consents viewer_consent
            ON viewer_consent.user_id = viewer_user.id
           AND viewer_consent.purpose = 'matching_gender_preferences'
           AND viewer_consent.withdrawn_at IS NULL
          JOIN users owner_user ON owner_user.id = photo.user_id AND owner_user.status = 'active'
          JOIN profiles owner_profile
            ON owner_profile.user_id = owner_user.id
           AND owner_profile.onboarding_completed
           AND owner_profile.location IS NOT NULL
          JOIN user_preferences owner_pref ON owner_pref.user_id = owner_user.id
          JOIN privacy_consents owner_consent
            ON owner_consent.user_id = owner_user.id
           AND owner_consent.purpose = 'matching_gender_preferences'
           AND owner_consent.withdrawn_at IS NULL
          WHERE viewer_user.id = sqlc.arg(viewer_id)
            AND viewer_user.status = 'active'
            AND viewer_profile.onboarding_completed
            AND viewer_profile.location IS NOT NULL
            AND cardinality(viewer_pref.genders) > 0
            AND cardinality(owner_pref.genders) > 0
            AND owner_user.gender = ANY(viewer_pref.genders)
            AND viewer_user.gender = ANY(owner_pref.genders)
            AND EXTRACT(YEAR FROM age(sqlc.arg(as_of)::timestamptz, owner_user.birth_date))::integer
                BETWEEN viewer_pref.min_age AND viewer_pref.max_age
            AND EXTRACT(YEAR FROM age(sqlc.arg(as_of)::timestamptz, viewer_user.birth_date))::integer
                BETWEEN owner_pref.min_age AND owner_pref.max_age
            AND ST_DWithin(
                viewer_profile.location,
                owner_profile.location,
                LEAST(viewer_pref.max_distance_km, owner_pref.max_distance_km)::float8 * 1000.0
            )
            AND NOT EXISTS (
                SELECT 1 FROM swipes swipe
                WHERE swipe.actor_id = viewer_user.id AND swipe.target_id = owner_user.id
            )
      )
  )
  AND NOT EXISTS (
      SELECT 1 FROM blocks block
      WHERE (block.blocker_id = sqlc.arg(viewer_id) AND block.blocked_id = photo.user_id)
         OR (block.blocker_id = photo.user_id AND block.blocked_id = sqlc.arg(viewer_id))
  );

-- name: TouchUserActivity :exec
UPDATE users
SET last_active_at = sqlc.arg(active_at)
WHERE id = sqlc.arg(user_id)
  AND last_active_at < sqlc.arg(active_at)::timestamptz - interval '15 minutes';
