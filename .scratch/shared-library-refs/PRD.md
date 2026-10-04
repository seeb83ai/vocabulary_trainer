# PRD: Shared library with per-user references

Status: needs-triage

## Problem Statement

When a learner imports a word list (for example HSK 3.0 level 1), the app copies every
vocabulary entry to the learner. For each entry it copies the zh word, one word row per
dictionary gloss, one link per gloss, and one SM-2 progress row per word (zh **and**
every gloss). A full HSK import writes about 7 rows per entry.

This causes three problems:

1. **Size and scale.** Each learner holds a private copy of data that is the same for
   everybody. The data grows linearly with the number of learners.
2. **Import speed.** On the production Raspberry Pi, an import must write tens of
   thousands of rows before the learner has the full list.
3. **No propagation.** A fix in the library or in the source dictionaries (CC-CEDICT,
   HanDeDict) never reaches learners who imported before the fix. When the open-source
   dictionaries publish a new version, we cannot give it to existing learners.

Measured on the production DB (2026-10-04): 26 users, 79k words, 74k translation links,
78k SM-2 rows, 296k dictionary entries, 91.8 MB. One active learner holds most of the
data. Today the net size saving is about zero, because the library prefill adds about as
many rows as the migration removes. The size and speed gains grow with each new learner.
The main gain now is propagation.

## Solution

The library user (user 1) holds the full library: zh vocabulary entries, their list tags,
and their native-language glosses, built from the dictionaries. A learner does not get a
copy. The learner gets a **library reference**: the per-user SM-2 progress row for that
library entry. All reads show the library data, adjusted by the learner's own
**translation overrides** (added glosses, deleted glosses) and an optional **pinyin
override**.

When the dictionaries change, a refresh updates the library in place. Learners see the
change at once. If a learner has overrides on a changed entry, the import screen asks
them per list: keep my version or take the library version (bulk, with a per-entry
list to expand).

New list entries are not added on their own. The learner syncs a list on request from
the import screen. Entries the learner deleted stay deleted during a sync.

Entries without a dictionary entry (for example the learner's own sentence cards) stay
user-owned, as today.

## User Stories

### Import and sync

1. As a learner, I want a list import to finish in seconds, so that I can start training at once.
2. As a learner, I want imported entries to show the current library glosses, so that I always learn from the newest dictionary data.
3. As a learner, I want the import screen to show which lists I already imported, so that I know what I have.
4. As a learner, I want an "update" action on an imported list, so that I can add entries that the library added to the list after my import.
5. As a learner, I want the update to show how many new entries it will add before it adds them, so that my training queue does not grow by surprise.
6. As a learner, I want entries I deleted to stay deleted when I update a list, so that I do not have to delete them again.
7. As a learner, I want the update screen to tell me "N entries you removed earlier" with an option to include them again, so that I can undo an old delete.
8. As a learner, I want the import modes (include, review, known) to work as today, so that the import fits my level.
9. As a learner, I want the tags I apply at import to stay my own tags, so that I can rename or remove them.
10. As a learner, I want an entry that I already have to get the list tag instead of a second copy, so that I never see duplicates.
11. As a learner, I want the import screen to tell me which native languages I will see ("Translations: EN + DE · change in Settings"), so that I know where to change it.

### Languages

12. As a learner, I want to see library glosses only in my primary and secondary native language, so that I do not see languages I do not study.
13. As a learner, I want a change of my secondary language to apply to all my entries at once, so that I do not have to import again.
14. As a learner, I want my own added glosses to show in any case, so that my custom work never disappears.

### Custom translations and pinyin

15. As a learner, I want to add my own gloss to a library entry, so that I can learn the meaning that I need.
16. As a learner, I want to delete a library gloss from my view of an entry, so that wrong or rare meanings do not confuse me.
17. As a learner, I want to change a gloss, so that I can correct it for me (stored as delete + add).
18. As a learner, I want to override the pinyin of an entry, so that I can fix a reading for me.
19. As a learner, I want the quiz to accept exactly the glosses that I see, so that answer checking is consistent with the word list.
20. As a learner, I want my overrides to have no effect on other learners, so that my changes stay private.
21. As a learner, I want new library glosses to appear on entries that I edited, so that my edits do not cut me off from fixes.

### Library updates and conflicts

22. As a learner, I want to know when the library changed an entry that I customized, so that I can decide which version to keep.
23. As a learner, I want one choice per list ("keep all mine" or "take all library"), so that a big dictionary update does not need hundreds of clicks.
24. As a learner, I want to expand the list and choose per entry, with a diff of library vs. mine, so that I can keep the edits that matter.
25. As a learner, I want "take library" to keep my SM-2 progress, so that I do not lose my schedule.
26. As a learner, I want an entry that the dictionary dropped to stay in my vocabulary, so that my progress never disappears because of an upstream change.

### Own entries and subwords

27. As a learner, I want a manually added entry that has a dictionary entry to become a library reference, so that it also gets library fixes.
28. As a learner, I want a CSV-uploaded entry that has a dictionary entry to become a library reference with my CSV glosses as overrides, so that my upload is kept and still gets fixes.
29. As a learner, I want automatic subwords (for example 饭 from 吃饭) to be library references, so that a dictionary fix reaches them too.
30. As a learner, I want entries without a dictionary entry (for example my sentence cards) to stay my own, so that I can keep studying them as today.

### Delete

31. As a learner, I want to delete an entry, so that it leaves my training.
32. As a learner, I want a delete to remove my progress, overrides, tags, confusion pairs and mnemonic for that entry, so that a delete is a real delete.
33. As a learner, I want to add a deleted entry again by hand, so that I can change my mind (this clears the tombstone).

### Existing data

34. As the active learner, I want the migration to show me exactly the same glosses, pinyin, tags and progress as before, so that my training does not change.
35. As a dormant learner, I want my entries to switch to the clean library, so that I get the full and current dictionary data when I come back.
36. As a learner, I want my progress, confusion pairs, match-game history, usage events, mnemonic scenes and tags to move to the library entry, so that nothing gets lost.

### Operator

37. As the operator, I want a full DB backup before the migration, so that I can restore if something goes wrong.
38. As the operator, I want the migration to compare every user's data before and after and to roll back on a difference, so that a bug cannot silently change data.
39. As the operator, I want a CLI command that refreshes the library from a new CC-CEDICT or HanDeDict version, so that upstream fixes reach all learners.
40. As the operator, I want the refresh to keep entry IDs stable (upsert by text), so that learner references and overrides stay valid.
41. As the operator, I want the refresh to set a change marker only on entries whose gloss set really changed, so that learners see conflicts only where needed.
42. As the operator, I want the library to be read-only in the UI, so that nobody changes data for all learners by accident.
43. As the operator, I want the old copy code removed after the migration, so that there is only one import path.
44. As the operator, I want each slice to be deployable alone with no visible change before the import switch, so that I can check each step on the Pi with real data.

## Implementation Decisions

### Terms (new; add to CONTEXT.md)

- **Library**: the zh vocabulary entries, list tags and glosses owned by the library user (user 1). Read-only in the UI.
- **Library reference**: a learner's SM-2 progress row on a library entry. It is the only row that says "this learner has this entry".
- **Translation override**: a learner's `add` or `delete` of one gloss on one entry.
- **Tombstone**: a record that a learner deleted a library entry, read only by the list sync.

### ADR

- This design changes ADR-0003 ("each user's vocabulary entries, translations … are scoped to their user_id"). Library entries and their glosses become shared. Per-user state (progress, overrides, tags, confusions, mnemonics) stays scoped to `user_id`. Write ADR-0005 "Shared library with per-user references" that amends ADR-0003.

### Data model

- **SM-2 progress** primary key changes from the word id to (user, word). The row is the library reference. It gets `needs_review` (moved from the word) and a nullable `pinyin_override`.
- **Mnemonic scenes** key changes from the word id to (user, word).
- **Word tags** get a user column, so (user, word, tag). Tags stay global names.
- **Confusion pairs, match-game history, usage events** already have a user column. No change, apart from moving ids during the migration.
- SM-2 progress rows for gloss (EN/DE) words are no longer created. The quiz reads progress only for zh entries.
- New table **translation overrides**: (user, zh entry, gloss word, op = add | delete, created at). Custom gloss texts are user-owned word rows.
- New table **tombstones**: (user, library entry).
- Library zh entries get `library_updated_at` and a `library_removed` flag.
- One SQL view **user translations** (user, zh entry, gloss word, source, rank) = library links in the user's primary/secondary language − delete overrides + add overrides, plus the links of user-owned entries. Every translation read goes through this view. Overlay logic lives only in the view.

### Modules

- **Library store** (deep module): upsert library entries and glosses from the dictionaries (split senses, same rule as today), keep IDs stable, set the change marker, mark removed entries, create on-demand entries. Interface: refresh all, ensure entry for a zh text.
- **Reference store** (deep module): add references for many entries in one transaction (import, sync, on-demand), remove a reference with all per-user rows and write the tombstone, list tombstones for a list.
- **Override store** (deep module): add, delete, change gloss; set pinyin override; compute the effective gloss set; list conflicts (override older than `library_updated_at`) per list; resolve conflicts (keep mine / take library) per list or per entry.
- **Migration to references** (one-time): for each user and each zh entry with a dictionary entry: ensure the library entry, compute the overrides with the same sense split as the import, move all per-user rows to the library id, delete the copy and orphan gloss words. Faithful (with delete overrides) for users active in the last 7 days. Add overrides only for all other users. Before/after check per user of effective glosses, pinyin, tags and progress; any difference rolls back.
- **Import worker**: creates references instead of copies. Sync = same job with the tombstones skipped. The `import_langs` input goes away.
- **Read queries**: all queries that read glosses switch to the view. All queries that read progress, tags or mnemonics filter by user on the new keys instead of the word owner.
- **Write handlers**: word create/update/delete branch on "library reference" vs. "own entry". Writes as the library user return an error.
- **CLI**: new `refresh-library` command (run after `import-cedict`). `import-hsk` and `import-topics` keep writing list tags to the library.
- **Frontend**: import screen shows imported lists, the update action, the tombstone hint, the language hint, the conflict dialog (bulk per list, expandable per entry with diff). The word edit sheet writes overrides.
- **Removed**: `ImportTemplateWords` (dead code that copies the whole library), the old copy path in the import worker, the library user's old progress rows.

### API

- Import request drops `import_langs`. Import response unchanged (202 + job).
- New: list imported lists with sync status (new entries count, tombstone count, conflict count).
- New: start a sync for a list (optionally include tombstoned entries).
- New: get conflicts for a list; resolve conflicts (bulk or per entry).
- Existing word endpoints keep their shape. The word id in responses is the library entry id for references.

### Rollout (one PR per slice, each green and deployable)

1. Per-user keys (progress, mnemonic scenes, word tags), no behaviour change.
2. Overlay read path: overrides table + view, all gloss reads switched, no behaviour change.
3. Library prefill + `refresh-library`, invisible to learners.
4. Import as references + sync + tombstones + conflict dialog.
5. On-demand library for subwords, manual add, CSV upload.
6. Migration of existing users + removal of the old copy code.

## Testing Decisions

- Test external behaviour only: what a learner sees and what the quiz accepts, not which rows exist. Example: "after the library adds a gloss, the learner's word shows it" and not "a row exists in table X".
- Go tests use the standard library only and in-memory SQLite, as today.
- **Library store**: refresh is idempotent; IDs stay stable; change marker only on a real change; dropped entries are flagged, not deleted; sense split matches the import.
- **Override store**: effective glosses for add, delete, change; language filter; new library gloss appears on an edited entry; conflict detection and both resolutions; user isolation (user A's override is invisible to user B).
- **Reference store**: import, sync, tombstone skip, include again, delete removes all per-user rows.
- **Migration**: fixtures for an active user with custom glosses, pinyin override, sentence cards, confusion pairs, mnemonic; a dormant user; a user with EN only. Assert the before/after check passes, and assert it fails and rolls back when a difference is injected. Prior art: the existing migration tests in the migrate package.
- **Handlers**: import, sync, conflicts endpoints, and write rejection for the library user. Prior art: the import handler and import worker tests.
- **Quiz regression**: the quiz accepts every effective gloss and rejects a deleted one. Prior art: quiz handler tests.
- **E2E**: import a list, sync it, delete an entry and sync again, resolve a conflict in bulk and per entry. Prior art: the onboarding and import E2E specs. Capture PR screenshots for the import screen and conflict dialog.
- Slices 1 and 2 must pass the full existing suite with no test changes except new keys, which proves "no behaviour change".

## Out of Scope

- Editing the library in the UI (decided: read-only; fixes go through the source dictionaries).
- Automatic sync of new list entries (decided: on request only).
- Live library tags on learner entries (decided: tags stay the learner's own).
- More than two native languages per learner.
- Moving an entry out of a list in the learner's tags when the library moves it (list changes are rare).
- Sharing custom glosses between learners.

## Further Notes

- Evaluation SQL and results (2026-10-04): only the active learner has real custom glosses (about 370 on about 160 entries). Pinyin differs on 9 entries. Every library entry has at least one dictionary gloss. The heaviest evaluation query ran 7.8 s on the Pi, so a one-shot startup migration is acceptable.
- The migration must use the import's sense split (in Go), not SQL. An SQL comparison of whole definitions overstates the overrides a lot.
- Users with no gloss in their secondary language (for example EN only) get an empty secondary language during the migration.
- Every gloss read and the migration check depend on the view. Measure the view on the Pi with production-sized data in slice 2, before slice 4.
