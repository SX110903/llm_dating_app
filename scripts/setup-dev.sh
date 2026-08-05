#!/usr/bin/env sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
root=$(CDPATH= cd -- "$script_dir/.." && pwd)
env_file="$root/.env"

if [ -e "$env_file" ] && [ "${1:-}" != "--force" ]; then
  echo ".env ya existe. Usa --force solo si quieres reemplazarlo." >&2
  exit 1
fi

"$script_dir/generate-dev-keys.sh"

new_secret() {
  openssl rand -base64 32 | tr '+/' '-_' | tr -d '=\n'
}

umask 077
cat > "$env_file" <<EOF
APP_ENV=development
HTTP_ADDR=:8080
LOG_LEVEL=info
CORS_ALLOWED_ORIGINS=http://localhost:8080
POSTGRES_DB=llmatch
POSTGRES_USER=llmatch_admin
POSTGRES_ADMIN_PASSWORD=$(new_secret)
APP_DB_PASSWORD=$(new_secret)
MIGRATOR_DB_PASSWORD=$(new_secret)
REDIS_PASSWORD=$(new_secret)
JWT_PRIVATE_KEY_PATH=./secrets/dev/jwt_private.pem
JWT_PUBLIC_KEY_PATH=./secrets/dev/jwt_public.pem
EOF

echo "Entorno de desarrollo creado en $env_file (ignorado por Git)."
