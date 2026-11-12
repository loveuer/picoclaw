#!/usr/bin/env bash
# Build the sole published target; output matches pkg/updater's archive contract.
set -euo pipefail

if [[ $# -lt 1 || $# -gt 2 || ! "$1" =~ ^[a-zA-Z0-9][a-zA-Z0-9._+-]*$ ]]; then
    echo 'Usage: bash scripts/package-release.sh VERSION [OUTPUT_DIR]' >&2
    exit 1
fi
release_version="$1"
repo_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$repo_dir"
output_dir="${2:-dist}"
mkdir -p "$output_dir"
output_dir="$(cd "$output_dir" && pwd -P)"
stage_dir="$(mktemp -d "${TMPDIR:-/tmp}/picoclaw-release.XXXXXX")"
trap 'rm -rf "$stage_dir"' EXIT

make build GOOS=linux GOARCH=amd64 BINARY_NAME=picoclaw CMD_DIR=cmd/picoclaw VERSION="$release_version"
cp build/picoclaw-linux-amd64 "$stage_dir/picoclaw"
cp LICENSE "$stage_dir/LICENSE"
archive=picoclaw_Linux_x86_64.tar.gz
tar -czf "$output_dir/$archive" -C "$stage_dir" picoclaw LICENSE
(
    cd "$output_dir"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$archive" > checksums.txt
    else
        shasum -a 256 "$archive" > checksums.txt
    fi
)
echo "Release artifacts: $output_dir/$archive and $output_dir/checksums.txt"
