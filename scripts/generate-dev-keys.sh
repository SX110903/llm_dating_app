#!/usr/bin/env sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
output_dir=${1:-"$script_dir/../secrets/dev"}

mkdir -p "$output_dir"
umask 077
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:3072 -out "$output_dir/jwt_private.pem"
openssl pkey -in "$output_dir/jwt_private.pem" -pubout -out "$output_dir/jwt_public.pem"

echo "Claves RSA de desarrollo creadas en $output_dir"

