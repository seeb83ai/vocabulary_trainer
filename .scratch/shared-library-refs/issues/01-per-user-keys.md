# Per-user keys for SM-2 progress, mnemonic scenes and word tags

Status: wontfix

Dropped 2026-10-04: the library reference is a slim learner zh word row
(`library_word_id`), so per-user tables keep their word-id keys. ADR-0005 and the
CONTEXT.md terms move to slice 02.
Type: AFK

## Parent

`.scratch/shared-library-refs/PRD.md`

## What to build

Prepare the schema for shared library entries without any visible change. SM-2 progress,
mnemonic scenes and word tags get an explicit user key, so that two learners can later
hold per-user state on the same vocabulary entry. `needs_review` moves from the word to
the SM-2 progress row. A nullable `pinyin_override` is added to the progress row (not
used yet). Progress rows for gloss (EN/DE) words are no longer created or read.

Also write ADR-0005 "Shared library with per-user references" (amends ADR-0003) and add
the new terms (library, library reference, translation override, tombstone) to CONTEXT.md.

## Acceptance criteria

- [ ] Migration rebuilds `sm2_progress` with primary key (user_id, word_id), filled from the word owner
- [ ] Migration rebuilds `hmm_scenes` with key (user_id, word_id)
- [ ] Migration adds `user_id` to `word_tags`, filled from the word owner
- [ ] `needs_review` lives on `sm2_progress`; `pinyin_override` column exists
- [ ] Gloss words get no progress rows; existing ones are removed
- [ ] All queries filter progress, mnemonic scenes and word tags by the user key, not by the word owner
- [ ] Migration test with pre-migration fixtures
- [ ] Full existing Go, JS and E2E suites pass with no behaviour change
- [ ] ADR-0005 and CONTEXT.md terms added

## Blocked by

None - can start immediately
