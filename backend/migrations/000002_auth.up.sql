CREATE TABLE users (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email               citext NOT NULL,
    password_hash       text NOT NULL,
    display_name        varchar(100),
    birth_date          date,
    gender              text,
    status              text NOT NULL DEFAULT 'active',
    email_verified_at   timestamptz,
    password_changed_at timestamptz NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT users_status_check CHECK (status IN ('active', 'suspended', 'deleted'))
);

CREATE UNIQUE INDEX users_email_uq ON users (email);
CREATE INDEX users_status_idx ON users (status);
CREATE INDEX users_discover_idx ON users (gender, birth_date) WHERE status = 'active';

CREATE TABLE refresh_tokens (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       uuid NOT NULL REFERENCES users (id),
    family_id     uuid NOT NULL,
    token_hash    bytea NOT NULL,
    replaced_by   uuid REFERENCES refresh_tokens (id),
    device_label  text,
    user_agent    text,
    ip            inet,
    expires_at    timestamptz NOT NULL,
    last_used_at  timestamptz,
    revoked_at    timestamptz,
    revoke_reason text,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX refresh_tokens_hash_uq ON refresh_tokens (token_hash);
CREATE INDEX refresh_tokens_user_active_idx ON refresh_tokens (user_id, revoked_at, expires_at);
CREATE INDEX refresh_tokens_family_idx ON refresh_tokens (family_id);
