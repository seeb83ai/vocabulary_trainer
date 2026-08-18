#!/usr/bin/env bash
#
# CI check: PRs that change service/frontend/ files should also add/update at
# least one screenshot under pr-screenshots/ (see e2e/helpers/screenshot.js
# and the "PR screenshots for frontend changes" rule in CLAUDE.md). Best
# effort: for a handful of page-specific frontend files, also nudges when no
# screenshot filename mentions that page — this is a naming heuristic, not a
# hard requirement, since screenshot names are free-form per PR.
#
# Usage: scripts/check-pr-screenshots.sh <base-ref> <head-ref>
set -euo pipefail

cd "$(dirname "$0")/.."

base="${1:?usage: $0 <base-ref> <head-ref>}"
head="${2:?usage: $0 <base-ref> <head-ref>}"

changed="$(git diff --name-only --diff-filter=ACMR "$base...$head")"

frontend_changed="$(echo "$changed" | grep -E '^service/frontend/' || true)"
if [ -z "$frontend_changed" ]; then
  echo "No service/frontend/ changes — nothing to check."
  exit 0
fi

screenshots_changed="$(echo "$changed" | grep -E '^pr-screenshots/.*\.png$' || true)"
if [ -z "$screenshots_changed" ]; then
  echo "FAIL: service/frontend/ changed but no pr-screenshots/*.png was added or updated." >&2
  echo "Frontend files changed:" >&2
  echo "$frontend_changed" | sed 's/^/  /' >&2
  echo >&2
  echo "See the 'PR screenshots for frontend changes' rule in CLAUDE.md:" >&2
  echo "  1. Add captureForPR(page, '<name>') to the e2e test for this change." >&2
  echo "  2. make screenshots FILE=e2e/<spec>.spec.js" >&2
  echo "  3. git add/commit/push, then ./scripts/pr-screenshot-url.sh pr-screenshots/<name>.png" >&2
  echo "     and paste its output into the PR description." >&2
  echo "  If this change has no visual/rendered-output difference, ignore this check." >&2
  exit 1
fi

echo "OK: screenshot(s) updated:"
echo "$screenshots_changed" | sed 's/^/  /'

# Best-effort per-page nudge — page-specific frontend file -> expected keyword
# in a changed screenshot's filename. Purely informational: naming is free-form.
declare -A page_keywords=(
  [train.js]=train
  [vocab.js]=vocab
  [stats.js]=stats
  [pinyin.js]=pinyin
  [mnemonics.js]=mnemonic
  [hmm-builder.js]=mnemonic
  [mismatches.js]=mismatch
  [settings.js]=settings
)

screenshots_lower="$(echo "$screenshots_changed" | tr '[:upper:]' '[:lower:]')"
while IFS= read -r f; do
  base_name="$(basename "$f")"
  keyword="${page_keywords[$base_name]:-}"
  [ -z "$keyword" ] && continue
  if ! echo "$screenshots_lower" | grep -q "$keyword"; then
    echo "NOTE: $f changed but no screenshot filename mentions '$keyword' — double-check the right page was captured." >&2
  fi
done <<< "$frontend_changed"

exit 0
