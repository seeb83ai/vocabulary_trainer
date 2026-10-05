# ADR-0005: Shared library with per-user references

## Status

Accepted (amends ADR-0003)

## Context

ADR-0003 scopes every user's vocabulary entries **and translations** to their `user_id`.
A list import therefore copied each library entry to the learner: the zh word, one word
row per gloss, one link per gloss and progress rows. That has three problems: the data
grows with every learner, an import must write tens of thousands of rows, and a fix in
the library or in the source dictionaries (CC-CEDICT, HanDeDict) never reaches learners
who imported before the fix.

## Decision

The library user (user 1) holds the shared library: zh entries, list tags and glosses
built from the dictionaries. The library is read-only in the UI.

A learner gets a **library reference**: one slim zh word row of their own with
`library_word_id` pointing to the library entry. The reference has no copied gloss
links. All per-user state (SM-2 progress, word tags, mnemonic scenes, confusion pairs,
match-game history, usage events) keeps hanging off the learner's own word row, so it
stays scoped to `user_id` as ADR-0003 requires.

A learner's glosses for a reference are computed by the `user_translations` view:

- library links in the learner's primary or secondary native language,
- minus the glosses the learner deleted (`translation_deletions`),
- plus the learner's own links on the reference row (added glosses, source `user`).

Every per-user gloss read uses the view. The view is a single `SELECT` without `UNION`,
so SQLite flattens it into the calling query and uses the `zh_word_id` index. (A
`UNION ALL` view was measured at 62 s for one word-list sort on production-sized data.)

The pinyin is copied to the reference when it is created. A learner edits it like the
pinyin of an own entry. A dictionary refresh changes glosses only.

Entries without a dictionary entry (for example sentence cards) stay user-owned copies.

## Consequences

- ADR-0003 still holds for all per-user state. Only library glosses are shared.
- A dictionary refresh must upsert library rows by text, so ids stay stable. It must
  never delete a library entry that learners reference; it flags it `library_removed`.
- Library changes reach every reference, also an edited one: the learner's
  overrides stay on top of the new library glosses. There are no conflicts to
  resolve. A learner who wants the library version back uses "reset to library"
  on the entry; the edit sheet first shows what the reset removes and brings
  back (glosses and pinyin). `library_updated_at` and `overrides_updated_at`
  stay as change markers, but no logic depends on them.
- Code that writes glosses must branch on "reference" vs. "own entry".
- Global lookups that are not per learner (for example the list of translation
  languages) may still read the `translations` table directly.
