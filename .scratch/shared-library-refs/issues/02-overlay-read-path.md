# Overlay read path: translation overrides and the user translations view

Status: done
Type: AFK

## Parent

`.scratch/shared-library-refs/PRD.md`

## What to build

Add `library_word_id` to words (nullable; set = library reference). Add `translation_deletions` (learner zh word, library gloss word) and one SQL view `user_translations` that every gloss read uses. For a
library reference (learner row with `library_word_id`) the view returns library links in the learner's primary/secondary
native language, minus deleted glosses, plus the learner's own links on the reference (added glosses). For a user-owned entry it
returns the entry's own links. All queries that read glosses (word list, quiz answer
check, confusion detection, match game, mismatches, stats, hanzi lookups, export) switch
to the view. Nothing changes visibly, because no library references exist yet.

## Acceptance criteria

- [x] `library_word_id` column, deletions table and view created by migration
- [x] Pinyin: copied to the reference at creation (decision during implementation), so pinyin reads do not change
- [x] ADR-0005 and CONTEXT.md terms (library, library reference, translation override, tombstone)
- [x] Every gloss read in the DB package goes through the view
- [x] Unit tests for the view: own entry, library ref, add, delete, language filter, user isolation
- [x] Existing suites pass unchanged
- [x] View timing measured on a production-sized copy (report in the PR)

## Blocked by

None - can start immediately
