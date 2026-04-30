# CONTEXT.md — Vocabulary Trainer

## Purpose

A shared, hosted web application for learning Chinese vocabulary using spaced repetition. Users sign up with an email address and study Chinese lexical units paired with translations in their native language. Chinese is always the target language being learned; the native-language side (English, German, or others) is always the prompt or answer reference.

## Core domain terms

**Vocabulary entry**
Any Chinese lexical unit — a single character, a multi-character word, a phrase, an idiom, or a full sentence — paired with one or more translations in a native language. The canonical unit in the system. Never call it just "word" when precision matters; a vocabulary entry may span multiple characters or words.

**Native language**
The non-Chinese side of a vocabulary entry. Currently English (EN) and German (DE); designed to support additional languages. Each user configures a primary native language and an optional secondary language. The system always presents Chinese as the quiz target.

**SM-2 progress**
Per-user, per-entry scheduling state that drives spaced repetition. Governs the next due date, ease factor, and repetition interval for each entry. Based on the SM-2 algorithm with fixed calibrated parameters (`QualityCorrect = 4`, `QualityWrong = 0`) — these values must not be adjusted speculatively.

**Learning phase**
The initial drill period for a newly introduced vocabulary entry. Cards in the learning phase appear at short retry intervals (minutes, not days). A card graduates from the learning phase after 3 correct answers in a row; wrong answers reset the streak. Tracked by the `learning_new_word` flag on `sm2_progress`.

**Component**
A hanzi radical or sub-character part shown during vocabulary entry introduction as a memory aid. Components help users memorise characters by breaking them into recognisable parts. They are not independently quizzed — the primary learnable unit is always the vocabulary entry (the zh word), not its components.

**Tag**
A user-defined organisational label (e.g. `HSK1`, `food`, `travel`) attached to vocabulary entries. Tags filter which entries appear in training and on the vocabulary list. Tags are purely organisational — they never affect SM-2 scheduling parameters or quiz difficulty.

**Quiz mode**
The direction and format of a single review card. Four modes exist:
- **EN → ZH** — translation prompt, Chinese answer
- **ZH → EN** — Chinese prompt, translation answer
- **ZH + Pinyin → EN** — Chinese + pinyin prompt, translation answer
- **Progressive** — auto-selects direction based on the user's accuracy and attempt count for that entry

**Daily new-word cap**
A per-instance limit on how many brand-new vocabulary entries are introduced per day (default: 5, set via `MAX_NEW_WORDS`). Once the cap is reached, only previously-seen cards are served for the rest of the day. Prevents cognitive overload.

**Pinyin training**
A separate learning mode for tone and sound discrimination. Users hear a pinyin syllable and identify it. Tracked by its own SM-2 progress table (`pinyin_progress`), independent of vocabulary entry progress.

**Mnemonic (HMM)**
A Hanzi Movie Method scene linking a character to an actor (pinyin initial), location (pinyin final), room (tone), and prop (radical). Mnemonics are memory aids — they appear during training to support recall but are not quizzed independently. A user builds their personal mnemonic library at `/mnemonics`.

**Confusion pair**
A recorded instance where a user's wrong answer is a valid translation of a *different* known vocabulary entry. Tracked to surface systematic mix-ups. Visible on `/mismatches`.

## Architecture invariants

- Chinese (`language = 'zh'`) is the fixed pivot language. All quiz prompts are zh vocabulary entries. Pinyin training, character components, and mnemonics are Chinese-specific features and will not be generalised to other languages.
- SM-2 progress is always tracked on the zh vocabulary entry. `word_id` in quiz responses always refers to the zh entry, never a translation.
- `GetNextCard` must always filter `WHERE w.language = 'zh'`.

## What this app is not

- Not a general-purpose language learning platform — Chinese is a hard constraint, not a configuration option.
- Not gamified — no points, streaks, leaderboards, or goal-setting (as of writing).
- Not an SRS research tool — SM-2 parameters are frozen; the app is optimised for practical daily use, not algorithm experimentation.
