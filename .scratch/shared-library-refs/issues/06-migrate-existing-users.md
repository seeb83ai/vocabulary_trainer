# Migrate existing users to library references

Status: needs-triage
Type: HITL (run on the Pi with a backup)

## Parent

`.scratch/shared-library-refs/PRD.md`

## What to build

A one-shot startup migration converts every user's own zh entries that have a dictionary
entry into library references: it sets `library_word_id`, computes overrides with the
import's sense split, clears pinyin equal to the library pinyin, and deletes the copied
links and orphan gloss words. Word ids do not change, so no per-user rows move. Users active in the last 7 days get a faithful migration (add and delete
overrides). All other users get add overrides only. Users with no gloss in their
secondary language get an empty secondary language. Before and after, it compares every
user's effective glosses, pinyin, tags and progress, and rolls back on any difference
(dormant users: only additions allowed). The migration copies the DB file first.
Finally, remove `ImportTemplateWords` and the old copy path.

## Acceptance criteria

- [ ] DB file backup before the migration runs
- [ ] Fixtures: active user with custom glosses, pinyin override, sentence cards, confusions, mnemonic; dormant user; EN-only user
- [ ] Check passes on fixtures; an injected difference makes it roll back
- [ ] Old copy code removed
- [ ] Operator runbook in the PR: backup, deploy, verify, restore

## Blocked by

- 05-on-demand-library
