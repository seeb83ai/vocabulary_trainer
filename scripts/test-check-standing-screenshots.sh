#!/usr/bin/env bash
#
# Tests for scripts/check-standing-screenshots.sh. Builds a throwaway git repo
# per case, commits a change on a branch and runs the check against main.
#
# Usage: scripts/test-check-standing-screenshots.sh
set -uo pipefail

script="$(cd "$(dirname "$0")" && pwd)/check-standing-screenshots.sh"
failures=0

# run_case <name> <expected-exit> <expected-stderr-substring-or-empty> <file>...
run_case() {
  local name="$1" want_exit="$2" want_msg="$3"
  shift 3
  local dir out got_exit
  dir="$(mktemp -d)"
  (
    cd "$dir"
    git init -q -b main
    git config user.email t@t && git config user.name t
    mkdir -p service/frontend/landing/teasers/010-training images
    touch README.md
    git add -A && git commit -q -m base
    git checkout -q -b change
    for f in "$@"; do
      mkdir -p "$(dirname "$f")"
      echo "$RANDOM" >> "$f"
    done
    git add -A && git commit -q -m change
  )
  # The script cd's to its own repo root, so copy it into the temp repo.
  mkdir -p "$dir/scripts" && cp "$script" "$dir/scripts/"
  out="$(cd "$dir" && ./scripts/check-standing-screenshots.sh main change 2>&1)"
  got_exit=$?
  rm -rf "$dir"

  if [ "$got_exit" -ne "$want_exit" ]; then
    echo "FAIL: $name — exit $got_exit, want $want_exit"; echo "$out" | sed 's/^/    /'
    failures=$((failures + 1)); return
  fi
  if [ -n "$want_msg" ] && ! echo "$out" | grep -qF "$want_msg"; then
    echo "FAIL: $name — output lacks '$want_msg'"; echo "$out" | sed 's/^/    /'
    failures=$((failures + 1)); return
  fi
  echo "ok:   $name"
}

run_case "no frontend change"            0 ""                        docs/notes.md
run_case "JS-only frontend change"       0 ""                        service/frontend/stats.js
run_case "css change, no images"         1 "images/"                 service/frontend/app.css
run_case "css change, no images (landing)" 1 "landing/teasers/"      service/frontend/app.css
run_case "html change, README only"      1 "landing/teasers/"        service/frontend/stats.html images/chinese_stats.png
run_case "html change, landing only"     1 "images/"                 service/frontend/stats.html service/frontend/landing/teasers/010-training/10-a.png
run_case "css change, both updated"      0 "OK"                      service/frontend/app.css images/chinese_stats.png service/frontend/landing/teasers/010-training/10-a.png
run_case "images only, no css/html"      0 ""                        images/chinese_stats.png

if [ "$failures" -ne 0 ]; then
  echo "$failures case(s) failed" >&2
  exit 1
fi
echo "all cases passed"
