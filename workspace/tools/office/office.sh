#!/usr/bin/env bash
# Invoke with bash; onboarding intentionally writes resources without executable bits.
set -euo pipefail
runtime_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
workspace_dir="$(cd -- "$runtime_dir/../.." && pwd -P)"

fail() { echo "office: $*" >&2; exit 1; }
case "$workspace_dir" in
    *,*) fail "Workspace paths containing commas are not supported by Docker bind mounts." ;;
esac
command -v docker >/dev/null 2>&1 || fail "Docker is missing. Install Docker and allow this user to access its daemon."
docker info >/dev/null 2>&1 || fail "Cannot access the Docker daemon. Check that Docker is running and this user has permission."

# Image identity follows the actual build inputs, so an updated runtime cannot silently use an old image.
if command -v sha256sum >/dev/null 2>&1; then
    runtime_hash="$(cat "$runtime_dir"/{Dockerfile,requirements.txt,package.json,package-lock.json,entrypoint.sh} | sha256sum)"
else
    runtime_hash="$(cat "$runtime_dir"/{Dockerfile,requirements.txt,package.json,package-lock.json,entrypoint.sh} | shasum -a 256)"
fi
image="picoclaw-office:${runtime_hash:0:16}"

run_office() {
    docker image inspect "$image" >/dev/null 2>&1 || fail "Runtime image is missing. Run: bash \"$runtime_dir/office.sh\" setup"
    # Mount only the workspace, never the host HOME or Docker socket. Do not evaluate user arguments.
    docker run --rm --init --network none --cap-drop ALL --security-opt no-new-privileges \
        --user "$(id -u):$(id -g)" --env HOME=/tmp/office-home \
        --mount "type=bind,source=$workspace_dir,target=$workspace_dir" \
        --workdir "$office_workdir" "$image" "$@"
}

# Preserve a caller's directory only inside this workspace; otherwise use its root.
office_workdir="$(pwd -P)"
case "$office_workdir/" in
    "$workspace_dir/"*) ;;
    *) office_workdir="$workspace_dir" ;;
esac

case "${1:-}" in
    setup)
        [[ $# == 1 ]] || fail "Usage: office.sh setup"
        docker build --tag "$image" "$runtime_dir"
        echo "office: runtime ready: $image"
        ;;
    check)
        [[ $# == 1 ]] || fail "Usage: office.sh check"
        run_office python "$runtime_dir/check.py"
        ;;
    run)
        shift
        [[ "${1:-}" == -- ]] || fail "Usage: office.sh run -- <command> [arguments...]"
        shift
        [[ $# -gt 0 ]] || fail "Provide a command after --"
        run_office "$@"
        ;;
    *) fail "Usage: office.sh {setup|check|run -- <command> [arguments...]}" ;;
esac
