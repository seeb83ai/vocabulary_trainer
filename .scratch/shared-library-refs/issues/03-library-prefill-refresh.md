# Library prefill and refresh-library CLI

Status: needs-triage
Type: AFK

## Parent

`.scratch/shared-library-refs/PRD.md`

## What to build

Fill the library user's zh entries with glosses from the dictionaries (EN from CC-CEDICT,
DE from HanDeDict), split into senses with the same rule as the import. A refresh
upserts by text: unchanged glosses keep their ids, new glosses are added, dropped glosses
lose their link. `library_updated_at` is set only on entries whose gloss set changed. A
library entry that loses all dictionary entries is flagged `library_removed`, never
deleted. A new `refresh-library` CLI runs the refresh; a startup migration runs the first
prefill. Learners see nothing yet.

## Acceptance criteria

- [ ] Library store: refresh-all and ensure-entry-for-text
- [ ] Refresh is idempotent; ids stay stable; change marker only on a real change
- [ ] Removed entries are flagged, not deleted
- [ ] Glosses get rank from the gloss rank cache and source `cedict`
- [ ] `refresh-library` CLI documented in README
- [ ] Old library user progress rows deleted

## Blocked by

None - can start immediately
