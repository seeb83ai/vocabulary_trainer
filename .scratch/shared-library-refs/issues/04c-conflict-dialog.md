# Conflict detection and dialog

Status: done, then replaced by "reset to library" (see the update below)
Type: HITL (UI review)

## Parent

`.scratch/shared-library-refs/PRD.md`

## What to build

When the library changed an entry after a learner's override on it, the import screen
shows a conflict count per list. A dialog offers "keep all mine" / "take all library",
and an expandable per-entry list with a diff (library now vs. mine). "Take library"
deletes the entry's overrides and keeps progress. "Keep mine" marks the overrides as
seen, so the conflict disappears.

Note (implementation): the dialog is an inline panel in Vocabulary → Import. The per-word diff shows what only the library and only the learner has now. Open for UI review: it does not show what the library itself changed.

## Acceptance criteria

- [x] Conflict = override older than the entry's `library_updated_at`
- [x] API: conflicts per list with diff; resolve bulk or per entry
- [x] Progress kept in both resolutions
- [x] Handler tests and E2E; PR screenshots of the dialog for review

## Blocked by

- 04a-import-as-library-references

## Update: replaced by "reset to library"

A UI review found that the conflict dialog adds little. Library changes reach
every reference, also an edited one, so "keep mine" only hid the notice and
"take library" only removed the learner's overrides. Decision:

- No conflicts, no Review link in Your lists, no conflict endpoints.
- The edit sheet of a library reference shows **Reset to library** when the
  reference differs from the library (glosses by text, or pinyin).
- The button opens a confirmation box that lists what the reset removes,
  what it brings back and the pinyin change. **Reset** applies it.
- API: `GET /api/words/{id}/library-diff`, `POST /api/words/{id}/reset-library`.
