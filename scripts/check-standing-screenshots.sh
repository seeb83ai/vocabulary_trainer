#!/usr/bin/env bash
#
# CI check: PRs that change how pages look (service/frontend/*.css or *.html)
# should also regenerate the standing screenshots — the README images in
# images/ and the landing-page teasers in service/frontend/landing/teasers/
# (see the "Redesigns update the standing screenshots" rule in CLAUDE.md).
# JS-only changes are not checked: they rarely change what a page looks like.
#
# Usage: scripts/check-standing-screenshots.sh <base-ref> <head-ref>
set -euo pipefail

cd "$(dirname "$0")/.."

base="${1:?usage: $0 <base-ref> <head-ref>}"
head="${2:?usage: $0 <base-ref> <head-ref>}"

changed="$(git diff --name-only --diff-filter=ACMR "$base...$head")"

visual_changed="$(echo "$changed" | grep -E '^service/frontend/[^/]+\.(css|html)$' || true)"
if [ -z "$visual_changed" ]; then
  echo "No service/frontend/ CSS or HTML changes — nothing to check."
  exit 0
fi

readme_changed="$(echo "$changed" | grep -E '^images/.*\.png$' || true)"
landing_changed="$(echo "$changed" | grep -E '^service/frontend/landing/teasers/.*\.png$' || true)"

missing=()
[ -z "$readme_changed" ] && missing+=("images/*.png (README) — run: make screenshots-readme")
[ -z "$landing_changed" ] && missing+=("service/frontend/landing/teasers/**/*.png (landing page) — run: make screenshots-landing")

if [ "${#missing[@]}" -gt 0 ]; then
  echo "FAIL: page styles or markup changed but these screenshots were not updated:" >&2
  printf '  %s\n' "${missing[@]}" >&2
  echo "Files changed:" >&2
  echo "$visual_changed" | sed 's/^/  /' >&2
  echo >&2
  echo "See the 'Redesigns update the standing screenshots' rule in CLAUDE.md." >&2
  echo "  If this change does not alter how existing pages look, ignore this check." >&2
  exit 1
fi

echo "OK: standing screenshots updated:"
echo "$readme_changed" "$landing_changed" | tr ' ' '\n' | sed 's/^/  /'
exit 0
