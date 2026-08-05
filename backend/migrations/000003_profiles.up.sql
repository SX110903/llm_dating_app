CREATE TABLE profiles (
    user_id              uuid PRIMARY KEY REFERENCES users (id),
    bio                  varchar(500),
    interests            text[] NOT NULL DEFAULT '{}',
    city                 varchar(120),
    location             geography(Point, 4326),
    questionnaire        jsonb NOT NULL DEFAULT '{}',
    onboarding_completed boolean NOT NULL DEFAULT false,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX profiles_location_gix ON profiles USING GIST (location);
CREATE INDEX profiles_onboarding_idx ON profiles (user_id) WHERE onboarding_completed;
CREATE INDEX profiles_interests_gin ON profiles USING GIN (interests);

CREATE TABLE user_preferences (
    user_id         uuid PRIMARY KEY REFERENCES users (id),
    min_age         smallint NOT NULL,
    max_age         smallint NOT NULL,
    max_distance_km smallint NOT NULL,
    genders         text[] NOT NULL DEFAULT '{}',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT user_preferences_age_range_check CHECK (min_age >= 18 AND min_age <= max_age AND max_age <= 100),
    CONSTRAINT user_preferences_distance_check CHECK (max_distance_km >= 1 AND max_distance_km <= 500)
);

-- RGPD art. 9: genders revela de facto orientación/preferencia sexual y se
-- trata como dato de categoría especial. Requiere consentimiento explícito,
-- separado y versionado (privacy_consents) antes de persistirse.
CREATE TABLE privacy_consents (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        uuid NOT NULL REFERENCES users (id),
    purpose        text NOT NULL,
    policy_version text NOT NULL,
    granted_at     timestamptz NOT NULL DEFAULT now(),
    withdrawn_at   timestamptz,
    source         text NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX privacy_consents_user_purpose_idx ON privacy_consents (user_id, purpose, granted_at DESC);
-- At most one active (non-withdrawn) consent per user and purpose.
CREATE UNIQUE INDEX privacy_consents_active_uq ON privacy_consents (user_id, purpose) WHERE withdrawn_at IS NULL;

CREATE TABLE photos (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users (id),
    storage_key text NOT NULL,
    mime_type   text NOT NULL,
    byte_size   bigint NOT NULL,
    width       integer NOT NULL,
    height      integer NOT NULL,
    position    smallint NOT NULL,
    is_primary  boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz,
    CONSTRAINT photos_byte_size_check CHECK (byte_size > 0),
    CONSTRAINT photos_dimensions_check CHECK (width > 0 AND height > 0),
    CONSTRAINT photos_position_check CHECK (position >= 0 AND position <= 5),
    CONSTRAINT photos_mime_type_check CHECK (mime_type IN ('image/jpeg', 'image/png', 'image/webp'))
);

CREATE UNIQUE INDEX photos_user_position_uq ON photos (user_id, position) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX photos_user_primary_uq ON photos (user_id) WHERE is_primary AND deleted_at IS NULL;
CREATE UNIQUE INDEX photos_storage_key_uq ON photos (storage_key);
