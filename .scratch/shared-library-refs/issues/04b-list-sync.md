# List sync with tombstones

Status: done
Type: AFK

## Parent

`.scratch/shared-library-refs/PRD.md`

## What to build

The import screen lists the lists the learner imported. Each list shows how many new
library entries it has since the import and offers "update". Update adds references for
the new entries and skips tombstoned entries. The screen shows "N entries you removed
earlier · include again" to add them back.

Note (implementation): the sync is a normal `POST /api/import` for the same list and tags; `include_removed` adds deleted words again. The list overview is `GET /api/import/lists`.

## Acceptance criteria

- [x] API: imported lists with new-entry and tombstone counts
- [x] API: sync a list, optionally including tombstoned entries
- [x] Sync never adds a tombstoned entry unless asked
- [x] Handler tests and E2E (import, delete one entry, library adds an entry, sync)

## Blocked by

- 04a-import-as-library-references
