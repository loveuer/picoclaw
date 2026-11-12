#!/usr/bin/env bash
set -euo pipefail
umask 022
mkdir -p "$HOME"
exec "$@"
