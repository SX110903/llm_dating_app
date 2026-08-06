ALTER TABLE users
    ADD COLUMN last_active_at timestamptz NOT NULL DEFAULT now();

CREATE INDEX users_last_active_idx ON users (last_active_at DESC) WHERE status = 'active';

CREATE TABLE swipes (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_id   uuid NOT NULL REFERENCES users (id),
    target_id  uuid NOT NULL REFERENCES users (id),
    action     text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT swipes_distinct_users_check CHECK (actor_id <> target_id),
    CONSTRAINT swipes_action_check CHECK (action IN ('like', 'dislike', 'superlike'))
);

CREATE UNIQUE INDEX swipes_pair_uq ON swipes (actor_id, target_id);
CREATE INDEX swipes_actor_daily_idx ON swipes (actor_id, created_at DESC);
CREATE INDEX swipes_target_like_idx ON swipes (target_id, action, created_at DESC);

CREATE TABLE matches (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_low_id  uuid NOT NULL REFERENCES users (id),
    user_high_id uuid NOT NULL REFERENCES users (id),
    matched_at   timestamptz NOT NULL DEFAULT now(),
    unmatched_at timestamptz,
    unmatched_by uuid REFERENCES users (id),
    CONSTRAINT matches_ordered_pair_check CHECK (user_low_id < user_high_id),
    CONSTRAINT matches_unmatch_state_check CHECK ((unmatched_at IS NULL) = (unmatched_by IS NULL)),
    CONSTRAINT matches_unmatched_by_participant_check CHECK (
        unmatched_by IS NULL OR unmatched_by = user_low_id OR unmatched_by = user_high_id
    )
);

CREATE UNIQUE INDEX matches_pair_uq ON matches (user_low_id, user_high_id);
CREATE INDEX matches_low_active_idx ON matches (user_low_id, matched_at DESC) WHERE unmatched_at IS NULL;
CREATE INDEX matches_high_active_idx ON matches (user_high_id, matched_at DESC) WHERE unmatched_at IS NULL;

CREATE TABLE blocks (
    blocker_id uuid NOT NULL REFERENCES users (id),
    blocked_id uuid NOT NULL REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (blocker_id, blocked_id),
    CONSTRAINT blocks_distinct_users_check CHECK (blocker_id <> blocked_id)
);

CREATE INDEX blocks_blocked_idx ON blocks (blocked_id, blocker_id);

CREATE TABLE reports (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    reporter_id uuid NOT NULL REFERENCES users (id),
    reported_id uuid NOT NULL REFERENCES users (id),
    reason      text NOT NULL,
    description text NOT NULL DEFAULT '',
    status      text NOT NULL DEFAULT 'pending',
    created_at  timestamptz NOT NULL DEFAULT now(),
    resolved_at timestamptz,
    CONSTRAINT reports_distinct_users_check CHECK (reporter_id <> reported_id),
    CONSTRAINT reports_reason_check CHECK (
        reason IN ('harassment', 'spam', 'inappropriate_content', 'impersonation', 'underage', 'other')
    ),
    CONSTRAINT reports_status_check CHECK (status IN ('pending', 'reviewing', 'resolved', 'dismissed')),
    CONSTRAINT reports_description_length_check CHECK (char_length(description) <= 1000),
    CONSTRAINT reports_resolution_state_check CHECK (
        (status IN ('resolved', 'dismissed')) = (resolved_at IS NOT NULL)
    )
);

CREATE INDEX reports_status_idx ON reports (status, created_at);
CREATE INDEX reports_reporter_target_idx ON reports (reporter_id, reported_id, created_at DESC);
