CREATE TABLE messages (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    match_id     uuid NOT NULL REFERENCES matches (id),
    sender_id    uuid NOT NULL REFERENCES users (id),
    client_nonce uuid NOT NULL,
    type         text NOT NULL,
    content      varchar(2000),
    storage_key  text,
    read_at      timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    deleted_at   timestamptz,
    CONSTRAINT messages_type_check CHECK (type IN ('text', 'image', 'gif')),
    -- Text carries content and no object key; media carries a key and no text.
    -- The pair can never contradict the declared type.
    CONSTRAINT messages_payload_coherence_check CHECK (
        (type = 'text' AND content IS NOT NULL AND char_length(content) > 0 AND storage_key IS NULL)
        OR (type IN ('image', 'gif') AND storage_key IS NOT NULL AND content IS NULL)
    )
);

-- Idempotency: a retried send with the same nonce collides instead of
-- duplicating the message.
CREATE UNIQUE INDEX messages_sender_nonce_uq ON messages (sender_id, client_nonce);

-- Keyset pagination over (created_at, id); never OFFSET.
CREATE INDEX messages_match_cursor_idx ON messages (match_id, created_at DESC, id DESC) WHERE deleted_at IS NULL;

CREATE INDEX messages_unread_idx ON messages (match_id, read_at) WHERE read_at IS NULL AND deleted_at IS NULL;

-- receiver_id is deliberately absent: the recipient is derived from the two
-- participants of the match, so it cannot drift out of sync with it.
