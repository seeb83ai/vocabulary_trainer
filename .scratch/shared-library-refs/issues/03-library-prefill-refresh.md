# Library prefill and refresh-library CLI

Status: done
Type: AFK

## Parent

`.scratch/shared-library-refs/PRD.md`

## What to build

Fill the library user's zh entries with glosses from the dictionaries (EN from CC-CEDICT,
DE from HanDeDict), split into senses with the same rule as the import. A refresh
upserts by text: unchanged glosses keep their ids, new glosses are added, dropped glosses
lose their link. `library_updated_at` is set only on entries whose gloss set changed. A
library entry that loses all dictionary entries is flagged `library_removed`, never
deleted. A new `refresh-library` CLI runs the refresh; the server runs the first prefill at
startup when the library has no glosses yet (a migration cannot, because the sense split
lives in the db package). `import-hsk` and `import-topics` refresh at the end. Learners see nothing yet.

## Acceptance criteria

- [x] Library store: refresh-all and ensure-entry-for-text
- [x] Refresh is idempotent; ids stay stable; change marker only on a real change
- [x] Removed entries are flagged, not deleted
- [x] Glosses get rank from the gloss rank cache and source `cedict`
- [x] `refresh-library` CLI documented in README
- [x] Old library user progress rows deleted

## Blocked by

None - can start immediately
