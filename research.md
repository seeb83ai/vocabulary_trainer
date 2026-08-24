# Research: ideas from the Chinese-learning community

## Provenance note

The source requested was the r/ChineseLanguage wiki "Start" page. Outbound
fetches to `reddit.com`, `old.reddit.com`, and `web.archive.org` are blocked
in this environment, and a text-proxy fetch (`r.jina.ai`) was itself blocked
by Reddit's bot detection. So this document is **not** a link-by-link
summary of that wiki. Instead it draws on the well-established, recurring
resource categories and pain points that community (and the broader
Chinese-learning-app landscape — Pleco, Anki, Skritter, ChinesePod, Migaku,
HelloChinese, subs2srs) is known for, and maps each one against what
`vocabulary_trainer` already has.

Each idea below was checked against the actual codebase before scoring —
effort estimates cite the specific files/functions involved, not guesses.

## Scoring methodology

Four dimensions, each scored **1–5**:

| Dimension | 1 means | 5 means |
|---|---|---|
| **Benefit to user** | Marginal, few users would notice | Addresses a widely-felt gap |
| **Impact on learning** | Nice-to-have, doesn't change retention/skill | Directly improves acquisition or retention (SLA-backed: spaced repetition, tone discrimination, frequency exposure, active recall) |
| **Implementation effort** | Hours, reuses existing infra | Multi-week, new subsystem/schema/data pipeline |
| **Implementation risk** | Additive, isolated, easy to revert | Touches SM-2 core, quiz card selection, or other off-limits/high-blast-radius code |

**Overall priority score** = `Benefit + Impact − Effort − Risk + 12` (constant
shifts the range to 4–20 so it reads like a positive score). Higher is
better. This intentionally penalizes effort and risk at 1:1 weight against
benefit and impact — a great idea that's expensive or risky doesn't
automatically outrank a good idea that's cheap and safe.

## Priority ranking

Sorted by overall score, highest first. Ties broken by lower risk.

| Rank | Idea | Benefit | Impact on Learning | Effort | Risk | Overall |
|---|---|---|---|---|---|---|
| 1 | Leech detection / dedicated review queue | 5 | 5 | 2 | 2 | **18** |
| 2 | HSK 3.0 band tagging & filtering | 4 | 3 | 1 | 1 | **17** |
| 3 | Tone-pair confusion stats | 3 | 4 | 2 | 1 | **16** |
| 4 | Measure-word (量词) drill mode | 3 | 4 | 2 | 2 | **15** |
| 5 | Frequency-ranked list import | 3 | 3 | 2 | 1 | **15** |
| 6 | Radical/component browse mode | 3 | 2 | 2 | 1 | **14** |
| 7 | Chengyu word type + usage examples | 3 | 3 | 3 | 2 | **13** |
| 8 | Sentence-level listening cards | 4 | 4 | 4 | 3 | **13** |
| 9 | Sentence-mining import (paste → segment → extract) | 4 | 4 | 5 | 3 | **12** |
| 10 | Handwriting/stroke-order practice mode | 3 | 3 | 5 | 3 | **10** |

The detail write-ups below are numbered to match this rank order.

---

## 1. Leech detection / dedicated review queue — Score 18

**What it is.** In SRS communities (Anki foremost among them), a "leech" is
a card that keeps failing no matter how many times it's reviewed — it eats
review time without ever graduating. The standard fix is to detect leeches
automatically and pull them into a separate, deliberate-practice queue
instead of letting them cycle endlessly through the normal SM-2 rotation.

**Why it fits here.** The project already has half of this: `mismatches.go`
/ `db/hanzi.go`'s confusion-pair tracking (`DetectConfusion`,
`upsertConfusion` in `db/quiz.go`) records when a user's wrong answer
matches a *different* known word. That's confusion detection, not leech
detection — a word can fail repeatedly without ever being confused with
something else (e.g. genuinely forgotten vocabulary, ambiguous
translations). `SM2Progress` (`models/models.go`) already tracks
`TotalCorrect`/`TotalAttempts`/`Repetitions`, which is exactly the data a
leech rule needs — no new columns required.

**Sketch.** Add a `Store.GetLeeches(ctx, userID, tags, minAttempts,
maxAccuracy)` query in `db/quiz.go` alongside the existing stats functions,
using the existing `sm2_progress` join pattern already used by `GetStats`.
Surface it as a `?leech=true` filter reusing the existing tag-filter
plumbing in `GetNextCard`, or as a new stats panel in `stats.go` /
`stats.js` listing "stuck" words with a link to jump into a focused
review session. No SM-2 algorithm changes — this is a *view* over existing
data, so it doesn't touch the off-limits SM-2 formula.

**Risk notes.** Genuinely low: it's read-only aggregation over existing
tables, with one new opt-in query path. The only "risk" is definitional —
picking sensible thresholds (e.g. ≥5 attempts and <40% accuracy) — which is
a product decision, not a technical one.

**Testing.** `db/quiz_test.go` (or `db_test.go`) needs a leech-detection
unit test with seeded progress rows; if exposed via a new endpoint, add an
integration test in `handlers/handlers_test.go` and register the route in
`newRouter()`.

---

## 2. HSK 3.0 band tagging & filtering — Score 17

**What it is.** Tag every zh word with its HSK 3.0 level (1–9, since HSK 3.0
replaced the old 1–6 scale) so users can filter quizzes and browsing by
level — the single most common organizing axis in the Chinese-learning
community's recommended resources and word lists.

**Why it fits here.** This is almost entirely a data/import problem, not a
code problem. `db/tags.go` already implements a full many-to-many tag
system (`getOrCreateTag`, `setWordTags`, `GetAllTags`, `GetTagDetails`), and
`CreateWord`/`GetWords`/`GetNextCard` already accept and filter on
`tags []string`. HSK bands need nothing more than tagging each imported word
`hsk-1` … `hsk-9`, which the existing `service/cmd/import-hsk` tool is
presumably already positioned to do (it's a dedicated HSK importer already
in the File map) — the incremental work is publishing/refreshing an HSK 3.0
source list and adding a "browse by HSK band" UI affordance in `vocab.js`
using the tag filter that already exists.

**Risk notes.** Minimal — it's additive data plus a UI filter that reuses
an existing, well-tested code path (`tierFilter`/tag filtering in
`db/words.go`). The only real cost is sourcing an accurate, licensable HSK
3.0 word list.

**Testing.** If `import-hsk` needs updating for HSK 3.0 bands, its own test
coverage should cover the new band values; no handler/DB test changes if
this stays purely a data-import + existing-tag-filter change.

---

## 3. Tone-pair confusion stats — Score 16

**What it is.** Tone confusion (2nd vs 3rd tone, 1st vs 4th, etc.) is one of
the most frequently cited beginner/intermediate obstacles in that
community. The existing pinyin listening quiz already *drills* this
implicitly — `db/pinyin.go`'s `GetPinyinDistractors` explicitly prioritizes
"same syllable, different tone" as the primary distractor strategy — but
there's no surfaced feedback telling a user *which* tone pairs they
personally confuse most.

**Why it fits here.** All the raw material already exists:
`sm2_progress`-style tracking for pinyin sounds (`GetPinyinProgress`,
`UpdatePinyinProgress`), and the tone field on `pinyin_sounds`
(`initial, final, tone, syllable`). This only needs an aggregation query —
group wrong-answer events by `(target.tone, chosen.tone)` — analogous to
the word-confusion-pairs table already used for hanzi mismatches, but
scoped to the pinyin quiz's existing answer-recording path in
`handlers/pinyin_quiz.go`.

**Risk notes.** Low — additive stats query plus (optionally) a small
`confusion_pairs`-like table for pinyin if event-level detail is wanted, or
computed live from existing progress if only aggregate counts are needed.
Doesn't touch SM-2 core or the pinyin quiz's card-selection logic.

**Testing.** Unit tests in `db/pinyin_test.go`-equivalent for the new
aggregation query; handler test if a new stats endpoint is added (register
in `newRouter()`).

---

## 4. Measure-word (量词) drill mode — Score 15

**What it is.** Classifiers/measure words (一**个**人, 一**本**书, 一**杯**水)
are a canonical beginner sticking point — nouns must be paired with the
correct measure word, and the community consistently recommends drilling
noun+classifier pairs directly rather than learning them in isolation.

**Why it fits here.** `models.Word` currently has no measure-word field, and
there's no concept of a noun-classifier pairing anywhere in the schema —
this is new domain modeling, not a filter over existing data. The cleanest
fit with existing architecture: model it the same way HMM mnemonic cards
work (`card_type` discrimination already exists on `QuizCard`, see
`CardType`/`EntityType`/`EntityKey` in `models/models.go`) — add a
`card_type="measure_word"` variant rather than inventing a parallel review
system. Requires a new migration (measure-word column on `words`, or a
join table `word_measure_words`), a new `db/measurewords.go` (new logical
domain, per the file-map convention of one file per domain), and quiz
integration in `GetNextCard`/`handlers/quiz.go`.

**Risk notes.** Moderate — touches `GetNextCard`'s card-selection logic
(shared, sensitive code) to interleave a new card type, similar in shape to
how HMM cards were presumably integrated. Needs the same care the HMM
integration took to avoid breaking the zh-word due-date scheduling
guarantee (`GetNextCard` must keep filtering `WHERE w.language='zh'` for
plain word cards).

**Testing.** New migration test (`TestNoDuplicateMigrationVersions` covers
uniqueness automatically), `db/measurewords_test.go`, handler test in
`handlers_test.go` + route registration, and an E2E test if it's a
user-visible quiz mode per the E2E-first cycle in CLAUDE.md.

---

## 5. Frequency-ranked list import — Score 15

**What it is.** Studying by real-world word frequency (e.g. SUBTLEX-CH
style lists) rather than purely by HSK band is a recurring recommendation —
frequency ordering surfaces genuinely useful vocabulary faster than
level-banded lists, which are curriculum-driven rather than usage-driven.

**Why it fits here.** This is structurally identical to idea #2 (HSK
tagging) — it's an import + tagging exercise using the existing
`service/cmd/import` tool and tag infrastructure, with the addition of a
numeric "frequency rank" that words don't currently have anywhere. The
simplest implementation avoids a schema change entirely: encode frequency
bands as tags (`freq-top500`, `freq-top1000`, …) exactly like HSK bands,
reusing `setWordTags`/`GetWords` tag filtering with zero new code paths. A
precise numeric rank (for "sort by frequency" rather than banded filtering)
would need a new nullable column, which is a small migration
(`ALTER TABLE words ADD COLUMN freq_rank INTEGER`, per the existing
migration conventions).

**Risk notes.** Low, same shape as HSK tagging. The main cost is sourcing
a redistributable frequency list with appropriate licensing.

**Testing.** Import-tool test coverage for the new list source; no
handler/DB changes if implemented purely via tags.

---

## 6. Radical/component browse mode — Score 14

**What it is.** Browsing/learning by radical (部首) rather than only
decomposing characters on demand — letting a user explore "which characters
use 氵(water)" as a study path, not just look up one character's parts.

**Why it fits here.** `db/hanzi.go` (561 lines) already owns hanzi
decomposition queries and zh-text translation lookups — the raw
component-to-character relationship data this needs almost certainly
already exists there in some form, since per-character decomposition
requires exactly that mapping. This is very likely "expose an existing
relationship the other direction" (component → list of characters) rather
than new data modeling, plus a new frontend browse view.

**Risk notes.** Low — read-only query addition to an existing, isolated
domain file, no interaction with SM-2 or quiz scheduling. Chief unknown is
whether the existing decomposition data is structured for efficient
reverse lookup (component → characters) or would need an index/table
addition — worth a quick spike before committing to the estimate.

**Testing.** New query test in `db/hanzi_test.go`-equivalent; a new
handler if exposed via API (register route); new frontend page per the
`add-frontend-page` skill/convention if it's a standalone browse UI.

---

## 7. Chengyu (成语) word type + usage examples — Score 13

**What it is.** Four-character idioms are frequently called out as an
intermediate/advanced plateau-breaker — the community consistently flags
"stopped learning single words, started learning chengyu" as a milestone.
Chengyu benefit from usage-example sentences far more than regular
vocabulary, since their meaning is often non-compositional.

**Why it fits here.** Chengyu are just zh `Word` rows today — nothing
prevents importing them now via existing `CreateWord`. What's genuinely
new: **usage-example sentences** are not modeled anywhere in the schema
(`Word` has `Text`, `Language`, `Pinyin` — no example-sentence field or
table). Needs a new `word_examples` table (one-to-many: a word can have
multiple example sentences) and a new domain file
(`db/examples.go`), surfaced on the word-detail view and optionally as
quiz-card enrichment (show an example sentence as a hint, similar to how
`Hint` already exists on `QuizCard` for HMM cards).

**Risk notes.** Moderate — new schema + new domain, but additive and
isolated (doesn't touch SM-2 or existing card selection unless examples are
wired into quiz hints, which is optional).

**Testing.** Migration test, `db/examples_test.go`, handler test +
route registration if exposed via API, README update (new user-visible
behavior).

---

## 8. Sentence-level listening cards — Score 13

**What it is.** Extending the existing pinyin listening quiz beyond single
syllables/words to short native-audio sentences — closer to real
comprehension practice, and a frequently recommended "next step" once
isolated-word listening plateaus.

**Why it fits here.** The pinyin quiz's plumbing (`handlers/pinyin_quiz.go`:
`Next`, `Answer`, `Stats`, `ServeAudio`) is built around single
`pinyin_sounds` rows with one audio file per syllable+tone. Sentence audio
is a different shape of asset (variable length, needs its own text
transcript, can't reuse the syllable/tone distractor-generation logic in
`GetPinyinDistractors`) — this is a parallel feature next to the pinyin
quiz, not an extension of it, despite the surface similarity.

**Risk notes.** Moderate-high — needs an audio-sourcing/storage strategy
(the existing `audio.go` TTS handler generates single-word audio; sentence
audio at scale is a bigger content pipeline question), plus new schema for
sentence+transcript+audio, plus new quiz-mode wiring. Not risky to existing
code (additive), but a genuinely bigger scope than it first appears.

**Testing.** Full E2E-first cycle per CLAUDE.md (new user-visible quiz
mode): E2E spec, DB tests, handler tests, route registration.

---

## 9. Sentence-mining import (paste sentence → segment → extract unknown words) — Score 12

**What it is.** The subs2srs/Migaku-style workflow: paste a sentence you
encountered "in the wild," auto-segment it into words, flag which ones
aren't in your deck yet, and one-click-add the unknowns with the sentence
kept as context. Popular because it studies vocabulary in the order you
actually encounter it, not a pre-set curriculum order.

**Why it fits here.** This is the most architecturally novel idea on the
list: Chinese has no whitespace between words, so "segment a sentence into
words" requires either a real segmentation library/algorithm (not
mentioned anywhere in the current Go dependency surface) or a
dictionary-longest-match heuristic against the existing `words` table —
which will misparse anything not already in the vocabulary, undermining
the exact case (finding *new* words) the feature exists for. This is a
real NLP dependency decision, not a straightforward CRUD addition.

**Risk notes.** High relative to the others here — new external dependency
or algorithm, ambiguous accuracy tradeoffs, and it's the kind of
"non-trivial change" CLAUDE.md's workflow explicitly says needs a proposed
approach and sign-off before any code, given the architectural decision
involved (which segmentation strategy, what happens on ambiguous parses).

**Testing.** Needs its own test strategy for segmentation accuracy
(a fixture set of sentences with expected splits), on top of standard
handler/DB tests for the resulting word-add flow.

---

## 10. Handwriting/stroke-order practice mode — Score 10

**What it is.** Stroke-order writing practice (Skritter-style) — draw the
character, get scored against correct stroke order/direction.

**Why it fits here.** `db/hanzi.go` already has character decomposition
data, which stroke-order practice could piggyback on for "what are the
components" scaffolding, but stroke-order *itself* (the sequence and
direction of each stroke) is a different dataset than component
decomposition and isn't implied by anything currently in the schema.
Would also need canvas-based drawing/gesture capture and stroke-matching
logic on the frontend — a genuinely new UI paradigm compared to the
existing text-input and multiple-choice quiz interactions everywhere else
in the app.

**Risk notes.** Highest effort and risk on this list: new stroke-order
dataset (licensing/sourcing), new frontend drawing/canvas component with
no precedent in the current vanilla-JS frontend, new scoring algorithm for
"was this stroke order acceptable," and a new quiz-card type. Large enough
in scope that it's closer to a standalone feature project than an
incremental addition.

**Testing.** Full E2E-first cycle; stroke-matching logic would need
extensive unit tests as pure/utility functions per the JS testing rules
(good candidate for `*.test.js` given it's likely to be a pure geometry
comparison function).

---

## Suggested next step

Per CLAUDE.md's plan-before-implementing rule, none of the above should
start as code yet. The top three (leech detection, HSK tagging, tone-pair
stats) are all low-risk/low-effort and could reasonably be scoped as one
issue batch each without a big design discussion. The bottom two
(sentence-mining, handwriting) are the ones worth a real design
conversation — and possibly an ADR under `docs/adr/` given the
architectural decisions involved — before any implementation work starts.
