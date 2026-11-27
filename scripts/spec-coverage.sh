#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
files=("$@")
if [ ${#files[@]} -eq 0 ]; then
  files=(spec/*.feature)
fi
tests="$(find . -name '*_test.go' -not -path './.git/*' -print0 | xargs -0 cat)"
missing=0
total=0
for f in "${files[@]}"; do
  while IFS= read -r title; do
    total=$((total + 1))
    if ! grep -qF "t.Run(\"$title" <<<"$tests"; then
      printf '%s: %s\n' "$(basename "$f")" "$title"
      missing=$((missing + 1))
    fi
  done < <(sed -nE 's/^[[:space:]]*Scenario( Outline)?:[[:space:]]*(.*[^[:space:]])[[:space:]]*$/\2/p' "$f")
done
printf '%d of %d scenarios have no matching test\n' "$missing" "$total" >&2
[ "$missing" -eq 0 ]
