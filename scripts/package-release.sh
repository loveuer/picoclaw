#!/usr/bin/env bash
# Build the published targets with archive names understood by pkg/updater.
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

build_dir="$stage_dir/build"
export GOCACHE="$(go env GOCACHE)"
export GOMODCACHE="$(go env GOMODCACHE)"
cp LICENSE "$stage_dir/LICENSE"

targets=(
    windows/amd64 windows/arm64
    darwin/amd64 darwin/arm64
    linux/amd64 linux/arm64
    android/arm64
)
archives=()
for target in "${targets[@]}"; do
    os=${target%/*}
    arch=${target#*/}
    case "$os" in
        windows) platform=Windows ;;
        darwin) platform=Darwin ;;
        linux) platform=Linux ;;
        android) platform=Android ;;
    esac
    archive_arch=$arch
    if [[ "$arch" == amd64 ]]; then
        archive_arch=x86_64
    fi
    binary_ext=
    if [[ "$os" == windows ]]; then
        binary_ext=.exe
    fi
    binary="picoclaw$binary_ext"
    archive="picoclaw_${platform}_${archive_arch}.tgz"
    make build GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 BUILD_DIR="$build_dir" VERSION="$release_version"
    cp "$build_dir/picoclaw-$os-$arch$binary_ext" "$stage_dir/$binary"
    COPYFILE_DISABLE=1 tar -czf "$stage_dir/$archive" -C "$stage_dir" "$binary" LICENSE
    archives+=("$archive")
done

(
    cd "$stage_dir"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "${archives[@]}" > checksums.txt
    else
        shasum -a 256 "${archives[@]}" > checksums.txt
    fi
    cp "${archives[@]}" checksums.txt "$output_dir/"
)
printf 'Release artifact: %s\n' "${archives[@]/#/$output_dir/}" "$output_dir/checksums.txt"
