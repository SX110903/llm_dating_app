#!/usr/bin/env sh
set -eu

: "${APP_DB_PASSWORD:?APP_DB_PASSWORD is required}"
: "${MIGRATOR_DB_PASSWORD:?MIGRATOR_DB_PASSWORD is required}"

psql --set=ON_ERROR_STOP=1 \
  --username "$POSTGRES_USER" \
  --dbname "$POSTGRES_DB" \
  --set=app_password="$APP_DB_PASSWORD" \
  --set=migrator_password="$MIGRATOR_DB_PASSWORD" <<'SQL'
CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public;
CREATE EXTENSION IF NOT EXISTS citext WITH SCHEMA public;
CREATE EXTENSION IF NOT EXISTS postgis WITH SCHEMA public;

SELECT format(
  'CREATE ROLE llmatch_migrator LOGIN PASSWORD %L NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT',
  :'migrator_password'
)
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'llmatch_migrator') \gexec

SELECT format(
  'CREATE ROLE llmatch_app LOGIN PASSWORD %L NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT',
  :'app_password'
)
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'llmatch_app') \gexec

REVOKE CREATE ON SCHEMA public FROM PUBLIC;
CREATE SCHEMA IF NOT EXISTS app AUTHORIZATION llmatch_migrator;
ALTER SCHEMA app OWNER TO llmatch_migrator;

GRANT CONNECT ON DATABASE :"POSTGRES_DB" TO llmatch_migrator, llmatch_app;
GRANT USAGE ON SCHEMA public TO llmatch_migrator, llmatch_app;
GRANT USAGE, CREATE ON SCHEMA app TO llmatch_migrator;
GRANT USAGE ON SCHEMA app TO llmatch_app;

ALTER ROLE llmatch_migrator IN DATABASE :"POSTGRES_DB" SET search_path TO app, public;
ALTER ROLE llmatch_app IN DATABASE :"POSTGRES_DB" SET search_path TO app, public;

ALTER DEFAULT PRIVILEGES FOR ROLE llmatch_migrator IN SCHEMA app
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO llmatch_app;
ALTER DEFAULT PRIVILEGES FOR ROLE llmatch_migrator IN SCHEMA app
  GRANT USAGE, SELECT ON SEQUENCES TO llmatch_app;
SQL
