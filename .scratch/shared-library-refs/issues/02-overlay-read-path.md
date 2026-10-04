# Overlay read path: translation overrides and the user translations view

Status: needs-triage
Type: AFK

## Parent

`.scratch/shared-library-refs/PRD.md`

## What to build

Add the translation overrides table (user, zh entry, gloss word, op add/delete,
created_at) and one SQL view `user_translations` that every gloss read uses. For a
library reference the view returns library links in the learner's primary/secondary
native language, minus delete overrides, plus add overrides. For a user-owned entry it
returns the entry's own links. All queries that read glosses (word list, quiz answer
check, confusion detection, match game, mismatches, stats, hanzi lookups, export) switch
to the view. Nothing changes visibly, because no library references exist yet.

## Acceptance criteria

- [ ] Overrides table and view created by migration
- [ ] Every gloss read in the DB package goes through the view
- [ ] Unit tests for the view: own entry, library ref, add, delete, language filter, user isolation
- [ ] Existing suites pass unchanged
- [ ] View timing measured on a production-sized copy (report in the PR)

## Blocked by

- 01-per-user-keys
