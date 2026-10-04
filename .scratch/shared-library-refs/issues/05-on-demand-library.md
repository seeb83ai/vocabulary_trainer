# On-demand library for subwords, manual add and CSV upload

Status: done
Type: AFK

## Parent

`.scratch/shared-library-refs/PRD.md`

## What to build

Any new zh entry whose text has a dictionary entry becomes a library reference. If the
library does not have it yet, it is created (untagged) from the dictionary first. This
applies to automatic subwords, manual "Add word" and CSV upload. Glosses the learner
gives that differ from the library become overrides. Entries without a dictionary entry
stay user-owned.

Note (implementation): `CreateWord` (manual add and CSV) and `createSubword` call `ensureLibraryWord` before their transaction. A word the learner already has as an own copy (from before the migration) stays own until slice 06 converts it.

## Acceptance criteria

- [x] Subwords, manual add and CSV create references when a dictionary entry exists
- [x] Learner-provided glosses become add/delete overrides
- [x] Sentence cards (no dictionary entry) stay user-owned
- [x] DB and handler tests; E2E for manual add and CSV

## Blocked by

- 04a-import-as-library-references
