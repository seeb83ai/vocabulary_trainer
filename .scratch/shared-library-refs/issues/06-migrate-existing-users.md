# Migrate existing users to library references

Status: implemented, waiting for the run on the Pi
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

Notes (implementation): the conversion runs at server start after the library prefill
(it needs the filled library and the Go sense split, so it is not a schema migration);
`data_conversions` records that it ran. Faithful = same gloss texts and sources: glosses
the learner typed (source `user`) stay their own links. Measured on synthetic data with
5,000 words: 2.6 s to create library words + 3.6 s to convert (after indexing
`translation_deletions.translation_word_id` and `word_game_shown.word_id`; without them
the gloss-word cleanup took 30 s). The rollback path is covered by a unit test of the
compare step.

## Acceptance criteria

- [x] DB file backup before the migration runs
- [x] Fixtures: active user with custom glosses, pinyin override, sentence cards, confusions, mnemonic; dormant user; EN-only user
- [x] Check passes on fixtures; an injected difference makes it roll back
- [x] Old copy code removed
- [x] Operator runbook in the PR: backup, deploy, verify, restore

## Blocked by

- 05-on-demand-library
