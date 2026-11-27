#!/usr/bin/env bash
set -euo pipefail

last=$(git describe --tags --abbrev=0 --match 'v[0-9]*' 2>/dev/null || true)
if [ -z "$last" ]; then
  echo "v0.1.0"
  exit 0
fi

range="$last..HEAD"
subjects=$(git log --format=%s "$range")
if [ -z "$subjects" ]; then
  exit 0
fi

breaking=$(git log --format=%B "$range" | grep -cE '^BREAKING[ -]CHANGE' || true)
if [ "$breaking" -gt 0 ] || grep -qE '^[a-z]+(\([^)]*\))?!:' <<<"$subjects"; then
  bump=major
elif grep -qE '^feat(\([^)]*\))?:' <<<"$subjects"; then
  bump=minor
elif grep -qE '^fix(\([^)]*\))?:' <<<"$subjects"; then
  bump=patch
else
  exit 0
fi

IFS=. read -r major minor patch <<<"${last#v}"
case "$bump" in
  major)
    if [ "$major" -eq 0 ]; then
      minor=$((minor + 1))
      patch=0
    else
      major=$((major + 1))
      minor=0
      patch=0
    fi
    ;;
  minor) minor=$((minor + 1)); patch=0 ;;
  patch) patch=$((patch + 1)) ;;
esac
echo "v$major.$minor.$patch"
