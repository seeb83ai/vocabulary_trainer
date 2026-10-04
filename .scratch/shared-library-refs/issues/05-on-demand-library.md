# On-demand library for subwords, manual add and CSV upload

Status: needs-triage
Type: AFK

## Parent

`.scratch/shared-library-refs/PRD.md`

## What to build

Any new zh entry whose text has a dictionary entry becomes a library reference. If the
library does not have it yet, it is created (untagged) from the dictionary first. This
applies to automatic subwords, manual "Add word" and CSV upload. Glosses the learner
gives that differ from the library become overrides. Entries without a dictionary entry
stay user-owned.

## Acceptance criteria

- [ ] Subwords, manual add and CSV create references when a dictionary entry exists
- [ ] Learner-provided glosses become add/delete overrides
- [ ] Sentence cards (no dictionary entry) stay user-owned
- [ ] DB and handler tests; E2E for manual add and CSV

## Blocked by

- 04a-import-as-library-references
