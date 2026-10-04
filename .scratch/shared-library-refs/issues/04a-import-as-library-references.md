# Import as library references

Status: needs-triage
Type: AFK

## Parent

`.scratch/shared-library-refs/PRD.md`

## What to build

A list import creates library references (one slim learner zh word row with
`library_word_id`, plus its SM-2 progress row) instead of copies. Import modes include/review/known and applied tags work as today. The learner's
word list, quiz, stats and edit sheet work on references. Editing glosses on a reference
writes translation overrides; editing pinyin writes the pinyin override. Deleting a
reference removes all per-user rows for the entry and writes a tombstone. Word writes as
the library user are rejected. The import screen drops the language checkboxes and shows
"Translations: EN + DE · change in Settings".

## Acceptance criteria

- [ ] Import creates references; no gloss words or links are copied
- [ ] Add/delete/change gloss on a reference = overrides; other learners unaffected
- [ ] Pinyin override shown everywhere pinyin shows
- [ ] Delete works as today (cascade) and writes a tombstone; manual add clears it
- [ ] Library user word writes return an error
- [ ] Handler and DB tests; E2E for import and edit of a reference

## Blocked by

- 02-overlay-read-path
- 03-library-prefill-refresh
