#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
if [ $# -eq 0 ]; then
  echo "real-notes: no path given, skipping"
  exit 0
fi
make build >/dev/null
go run ./scripts/realnotes --tkt "$root/bin/tkt" "$@"
