# Vocabulary Trainer — Promotion Findings & Competitive Gap Analysis

_Prepared 2026-08-17_

This document summarises two findings from a review of the vocabulary_trainer codebase (`README.md`, `CONTEXT.md`, `docs/adr/`):

1. Which existing features are strongest for promoting the app, with an emphasis on features that genuinely delight users ("Begeisterung"), and which of those competitors lack.
2. Which features popular competitor apps (Anki, Pleco, Skritter, Duolingo, Migaku/Du Chinese) offer that this app is currently missing.

**Bold** items flagged with 🚀 are differentiators not found in mainstream competitors.

---

## Part 1 — Features to Promote

### 1. Adaptive Learning Engine
- Progressive mode — 🚀 **auto-selects quiz direction based on the learner's live per-word accuracy and attempt count**. Anki decks are static; Duolingo's path is fixed for every user.
- Streak bonus — consecutive correct answers instantly boost a word's tier, and the bonus persists even after a later slip.
- Cycle mode — configurable 3-step direction rotation, with an "advance only on success" toggle.
- Five accuracy tiers (New / Struggling / Learning / Practicing / Mastered) drive scheduling and UI everywhere.

### 2. Motivation & Delight ("Begeisterung")
- Level-up interstitial — full-screen animated tier transition (🌰🌱🌿🌳🌸), opt-in, no dark patterns.
- Bucket growth icons on every result screen for an instant sense of progress.
- Word-matching mini-game — 🚀 **auto-triggers specifically from the learner's own recent confusion pairs**, turning weak spots into a game. No competitor gamifies mistakes this way.
- Wrong-answer forgiveness — "Accept as correct" for typos restores pre-answer SM-2 state (configurable: never / 1-char / always).
- Deliberately no points, leaderboards, or streak-pressure mechanics — delight without Duolingo-style manipulation.

### 3. Mistake Intelligence
- 🚀 **Confusion-pair tracking** — detects when a wrong answer is actually a valid translation of a different known word, surfaced immediately and in a dedicated `/mismatches` view. Close to unique among vocabulary trainers.
- Flexible answer matching — optional parenthesised segments, slash/comma alternatives — sharply cuts false negatives versus Anki's exact-match grading.
- Wrong-answer review with one-click "add my answer as an accepted translation," plus live pinyin display.
- Difficult-words drill — auto-flags the hardest words (half by accuracy, half by ease factor) for a targeted on-demand session.

### 4. Anti-Overwhelm Guardrails
- Daily new-word cap with a live "New today: X / Y" counter.
- 🚀 **Baseline gates** — pause new-word introduction when due-count, Struggling, Learning, or New-bucket counts exceed user-set thresholds.
- Cooldown between new words, skip-for-7-days, skip-for-today, and session extension to avoid repeating a card immediately.
- This entire cluster is close to unique — Anki simply dumps the review pile; Duolingo has no equivalent throttle.

### 5. Deep Chinese-Specific Tooling
- Character breakdown — radical, etymology, and component meanings, sourced from makemeahanzi, built in (no separate dictionary app needed).
- 🚀 **Hanzi Movie Method mnemonic builder** — maps pinyin initial→actor, final→location, tone→room, radical→prop, with a personal library reused automatically across words and surfaced during training. A paid course technique built natively into the SRS loop.
- 🚀 **Pinyin listening trainer** — dedicated tone/sound discrimination SRS across ~1,600 syllable+tone combinations, with its own progress tracking and confusion detection. Addresses tones, the hardest part of Mandarin, which most vocab apps ignore.
- Free neural TTS — Microsoft Edge neural voice, cached MP3s, built into the Go binary, zero API key or cost, Web Speech fallback.

### 6. Insight & Stats
- Daily history charts, word-level percentile stats (avg/median/P95 of attempts, accuracy, ease factor), an accuracy-distribution chart, hardest/most-practiced tables, and a 30-day due-date forecast — deeper than Anki's built-in stats with no add-on required.

### 7. Fast Content Onboarding
- Auto-translate (DeepL), bidirectional, with automatic pinyin fill and multi-meaning splitting.
- HSK 1–6 auto-import from mandarinbean with automatic tagging.
- CSV/bulk text import and N:N word↔translation relationships (one word, multiple senses).

### 8. Ownership, Privacy & Trust
- 🚀 **Self-hosted**, MIT-licensed, deployable to a Raspberry Pi via a single `make release`, single-file SQLite with scripted nightly backups.
- No ads and no tracking beyond the instance's own internal usage table.
- Personal API keys encrypted client-derived (PBKDF2-SHA256 + AES-GCM); SSRF protection on user-supplied local LLM URLs.
- Production-grade hardening out of the box: CSP, per-route rate limiting, login lockout, request-size caps, graceful shutdown — rare in a self-hosted project.

### 9. Deep Personalisation
- Per-tier quiz-mode configuration, per-user daily-learning settings, primary/secondary native language (EN/DE, extensible), and cross-device filter sync.

### Headline pitch — strongest three
- **Confusion-pair intelligence + mistake-driven mini-game** — none of Anki, Duolingo, Pleco, or Skritter combine mistake detection with a reactive gamified drill.
- **Integrated Hanzi Movie Method mnemonic builder** — a paid third-party course technique built natively into the SRS training loop.
- **Anti-overwhelm guardrail system** — daily cap + baseline gates + cooldown, protecting learners from review-pile burnout.

---

## Part 2 — Competitive Gaps

What established competitor apps (Anki, Pleco, Skritter, Duolingo, Migaku / Du Chinese-style apps) offer that this app does not yet have.

### 1. Handwriting & Production Practice — biggest gap
- No stroke-order writing trainer (Skritter's core feature: draw the character, get graded on stroke order/direction/accuracy). This app has zero handwriting practice — only reading and typing.
- No animated stroke-order sequence on the character breakdown (Pleco/Skritter both show this; here it's static radical/etymology info only).

### 2. Full Dictionary & Lookup Tooling
- No standalone dictionary browser — users can only quiz words already added, with no general "look up any word/character" reference mode.
- No OCR camera lookup (point a phone at text for instant translation).
- No handwriting-recognition input for looking up an unknown character.
- No example sentences per word — the data model stores translations only, with no in-context usage.

### 3. Native Content / Immersion
- No graded reader or article library at HSK levels.
- No reading mode with tap-to-lookup over real text (subtitles, articles, documents).
- No browser extension for mining vocabulary from webpages or videos already being consumed.
- No podcast or video integration.

### 4. Speaking & Sentence-Level Listening
- No speech recognition or pronunciation scoring — the pinyin trainer covers isolated tone/syllable discrimination only; the app never grades spoken output.
- No listening practice at the sentence/dialogue level, only single syllables and single words.

### 5. Structured Beginner Curriculum
- No guided course path with grammar points or sentence-construction lessons.
- No grammar explanations at all — this is a vocabulary/character trainer, not a language course.

### 6. Content Sharing & Community
- No shared/marketplace decks — Anki's biggest network effect (millions of community decks) has no equivalent here; every user starts from zero or the built-in HSK import.
- No social features — no friends, leaderboards, or progress sharing (partly deliberate design per `CONTEXT.md`, but still a real engagement-loop gap versus Duolingo).
- Per ADR-0003, user data is strictly isolated by `user_id` — no mechanism exists yet for one user to export or share a vocab list or mnemonic library with another.

### 7. Platform Reach
- No native mobile app (iOS/Android) — self-hosted web app only, no offline mode, no home-screen widget, no push notifications.
- Review reminders depend entirely on the user opening the site; there is no background sync notification.

### 8. Extensibility
- No add-on/plugin ecosystem (Anki's is extensive: image occlusion, custom card types, third-party stats).
- No data portability to/from Anki's `.apkg` format, so switching costs exist in both directions.

### Recommendation
Given the app's existing strengths in hanzi/mnemonic tooling, stroke-order writing practice and per-word example sentences are the two gaps most in tension with what already works well — natural extensions rather than scope changes. Native mobile apps and community deck-sharing are much larger commitments (per `CONTEXT.md`'s "what this app is not" constraints) and warrant a deliberate scope decision rather than a default yes.
