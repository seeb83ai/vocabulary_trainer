#!/usr/bin/env bash
#
# Route-drift check: every API endpoint registered in service/main.go must be
# documented in the README API table. Catches routes that get added in code but
# silently left out of the docs.
#
# Page routes (HTML pages) and a small set of internal/sub-resource route groups
# that are intentionally not part of the public API reference are skipped.
set -euo pipefail

cd "$(dirname "$0")/.."
MAIN="service/main.go"
README="README.md"

# Leaf path literals from chi route registrations (handles r.With(...).Verb too).
paths="$(grep -oE '\.(Get|Post|Put|Delete|Patch)\("/[^"]*"' "$MAIN" \
  | grep -oE '"/[^"]*"' | tr -d '"' | sort -u)"

missing=0
while IFS= read -r p; do
  [ -z "$p" ] && continue
  case "$p" in
    # HTML page routes and the catch-all — not part of the API table.
    /|/vocab|/stats|/train|/mnemonics|/mismatches|/pinyin|/settings|/login|/impressum|"/*") continue ;;
    # Generic leaf used by multiple nested groups (list/create at a group root).
    /export|/context|/generate-scene|/hmm/generate-scene) continue ;;
    # Internal/sub-resource endpoints covered by Go handler tests, not part of
    # the public API reference: auth internals, component & mnemonic-quiz
    # training, import internals, pinyin-quiz internals, tag detail/meta.
    /auth/status|/logout|/quiz/record-time) continue ;;
    /component/*|/components/*) continue ;;
    /hmm-quiz/*) continue ;;
    /import/*) continue ;;
    /pinyin-quiz/*) continue ;;
    /tags/*) continue ;;
  esac
  if ! grep -qF "$p" "$README"; then
    echo "UNDOCUMENTED route (in $MAIN, not in $README API table): $p"
    missing=1
  fi
done <<< "$paths"

if [ "$missing" -ne 0 ]; then
  echo ""
  echo "Add the above route(s) to the API table in $README, or skip-list internal ones in $0."
  exit 1
fi
echo "Route-drift check passed: all API routes are documented."
