-- name: GetActiveParticipants :one
-- Authorization in a single statement: the match must exist, still be active,
-- include the viewer, and carry no block in either direction. Anything else
-- returns no row, so the caller cannot tell the cases apart.
SELECT match.id, match.user_low_id, match.user_high_id, match.matched_at
FROM matches match
WHERE match.id = sqlc.arg(match_id)
  AND match.unmatched_at IS NULL
  AND (match.user_low_id = sqlc.arg(viewer_id) OR match.user_high_id = sqlc.arg(viewer_id))
  AND NOT EXISTS (
      SELECT 1 FROM blocks block
      WHERE (block.blocker_id = match.user_low_id AND block.blocked_id = match.user_high_id)
         OR (block.blocker_id = match.user_high_id AND block.blocked_id = match.user_low_id)
  );

-- name: InsertMessage :one
-- ON CONFLICT DO NOTHING makes a replayed nonce a no-op instead of a
-- duplicate; an empty result tells the repository to fetch the stored row.
INSERT INTO messages (id, match_id, sender_id, client_nonce, type, content, storage_key)
VALUES ($1, $2, $3, $4, $5, sqlc.narg(content), sqlc.narg(storage_key))
ON CONFLICT (sender_id, client_nonce) DO NOTHING
RETURNING *;

-- name: GetMessageByNonce :one
SELECT * FROM messages WHERE sender_id = $1 AND client_nonce = $2;

-- name: ListMessagesBefore :many
-- Keyset pagination over (created_at, id) so concurrent inserts cannot create
-- gaps or duplicates while the client pages backwards.
SELECT * FROM messages
WHERE match_id = sqlc.arg(match_id)
  AND deleted_at IS NULL
  AND (
      sqlc.narg(cursor_created_at)::timestamptz IS NULL
      OR created_at < sqlc.narg(cursor_created_at)::timestamptz
      OR (
          created_at = sqlc.narg(cursor_created_at)::timestamptz
          AND id < sqlc.narg(cursor_message_id)::uuid
      )
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit);

-- name: MarkMessagesRead :execrows
-- Only messages the viewer received are flagged, so a read receipt can never
-- be forged for one's own messages.
UPDATE messages
SET read_at = sqlc.arg(read_at)
WHERE match_id = sqlc.arg(match_id)
  AND sender_id <> sqlc.arg(viewer_id)
  AND read_at IS NULL
  AND deleted_at IS NULL
  AND created_at <= sqlc.arg(read_at);

-- name: ListConversations :many
SELECT
    match.id AS match_id,
    match.matched_at,
    other.id AS other_user_id,
    other.display_name,
    photo.id AS primary_photo_id,
    last_message.id AS last_message_id,
    last_message.sender_id AS last_message_sender_id,
    -- A match with no messages yet leaves the lateral join NULL; coalescing
    -- keeps the column non-null so it scans cleanly. last_message_id is what
    -- signals whether a last message actually exists.
    COALESCE(last_message.type, '')::text AS last_message_type,
    last_message.content AS last_message_content,
    last_message.created_at AS last_message_created_at,
    last_message.read_at AS last_message_read_at,
    COALESCE(unread.total, 0)::bigint AS unread_count
FROM matches match
JOIN users other ON other.id = CASE
    WHEN match.user_low_id = sqlc.arg(viewer_id) THEN match.user_high_id
    ELSE match.user_low_id
END
LEFT JOIN photos photo ON photo.user_id = other.id AND photo.is_primary AND photo.deleted_at IS NULL
LEFT JOIN LATERAL (
    SELECT m.id, m.sender_id, m.type, m.content, m.created_at, m.read_at
    FROM messages m
    WHERE m.match_id = match.id AND m.deleted_at IS NULL
    ORDER BY m.created_at DESC, m.id DESC
    LIMIT 1
) last_message ON true
LEFT JOIN LATERAL (
    SELECT count(*) AS total
    FROM messages m
    WHERE m.match_id = match.id
      AND m.sender_id <> sqlc.arg(viewer_id)
      AND m.read_at IS NULL
      AND m.deleted_at IS NULL
) unread ON true
WHERE (match.user_low_id = sqlc.arg(viewer_id) OR match.user_high_id = sqlc.arg(viewer_id))
  AND match.unmatched_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM blocks block
      WHERE (block.blocker_id = sqlc.arg(viewer_id) AND block.blocked_id = other.id)
         OR (block.blocker_id = other.id AND block.blocked_id = sqlc.arg(viewer_id))
  )
ORDER BY COALESCE(last_message.created_at, match.matched_at) DESC, match.id DESC
LIMIT sqlc.arg(page_limit);
