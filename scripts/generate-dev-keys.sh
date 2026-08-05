#!/usr/bin/env sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
output_dir=${1:-"$script_dir/../secrets/dev"}

go run "$script_dir/generate_dev_keys.go" -output "$output_dir"
