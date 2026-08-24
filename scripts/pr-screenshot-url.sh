#!/usr/bin/env bash
#
# Generate ready-to-paste PR-description markdown for screenshots captured via
# `make screenshots-pr` (see e2e/helpers/screenshot.js). PR/issue description text
# isn't tied to a git ref, so relative image paths don't render there — this
# builds the absolute raw.githubusercontent.com URL for the current branch and
# verifies it actually resolves (i.e. the file has been committed AND pushed)
# before printing the markdown line.
#
# Usage: scripts/pr-screenshot-url.sh pr-screenshots/stats-page.png [more.png ...]
set -euo pipefail

cd "$(dirname "$0")/.."

if [ "$#" -eq 0 ]; then
  echo "Usage: $0 <path/to/screenshot.png> [more.png ...]" >&2
  exit 1
fi

remote="$(git remote get-url origin)"
slug="$(echo "$remote" | sed -E 's#^git@github\.com:##; s#^https://github\.com/##; s#\.git$##')"
branch="$(git rev-parse --abbrev-ref HEAD)"

status=0
for path in "$@"; do
  if ! git ls-files --error-unmatch "$path" >/dev/null 2>&1; then
    echo "SKIP $path: not committed yet — git add/commit it first" >&2
    status=1
    continue
  fi
  if [ -n "$(git status --porcelain -- "$path")" ]; then
    echo "SKIP $path: has local changes not yet committed — commit and push first" >&2
    status=1
    continue
  fi

  url="https://raw.githubusercontent.com/${slug}/${branch}/${path}"
  code="$(curl -sS -o /dev/null -w '%{http_code}' "$url")"
  if [ "$code" != "200" ]; then
    echo "FAIL $path: $url returned $code — has the branch been pushed?" >&2
    status=1
    continue
  fi

  name="$(basename "$path" .png)"
  name="${name//-/ }"
  name="${name//_/ }"
  name="${name^}"
  echo "![${name}](${url})"
done

exit "$status"
