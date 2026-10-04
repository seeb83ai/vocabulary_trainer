# Conflict detection and dialog

Status: needs-triage
Type: HITL (UI review)

## Parent

`.scratch/shared-library-refs/PRD.md`

## What to build

When the library changed an entry after a learner's override on it, the import screen
shows a conflict count per list. A dialog offers "keep all mine" / "take all library",
and an expandable per-entry list with a diff (library now vs. mine). "Take library"
deletes the entry's overrides and keeps progress. "Keep mine" marks the overrides as
seen, so the conflict disappears.

## Acceptance criteria

- [ ] Conflict = override older than the entry's `library_updated_at`
- [ ] API: conflicts per list with diff; resolve bulk or per entry
- [ ] Progress kept in both resolutions
- [ ] Handler tests and E2E; PR screenshots of the dialog for review

## Blocked by

- 04a-import-as-library-references
