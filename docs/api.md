# HTTP API

All endpoints are under `/api` and use JSON. Except for the demo endpoints, they need a session cookie (see [Configuration](configuration.md#accounts-and-authentication)). Errors use the shape `{"error": "..."}`.

`scripts/check-route-drift.sh` checks in CI that every route in `service/main.go` is listed here.

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/demo/cards` | Public demo card list for the landing page (no auth, translations omitted) |
| `POST` | `/api/demo/answer` | Check a demo answer statelessly (no auth; returns `correct` + accepted translations) |
| `POST` | `/api/login` | Sign in with email and password; sets the session cookie. Five wrong passwords lock the account for 15 minutes |
| `POST` | `/api/register` | Create an account; sends a verification email when SMTP is configured |
| `GET` | `/api/verify-email` | Verify an email address with the token from the email |
| `POST` | `/api/change-password` | Change the password of the signed-in user |
| `GET` | `/api/quiz/next` | Get the next card to study (`mode`, `tags` query params; `difficult=true` serves only flagged difficult-drill words) |
| `POST` | `/api/quiz/answer` | Submit an answer (a correct answer clears the word's difficult-drill flag). Send `drill: true` in the difficult-words drill, so the answer does not count as a review. Send `pinyin_seen`, `audio_played` and `play_pressed` to tell what help the user had before the answer. The server counts the answer in its [answer buckets](development.md#answer-buckets). The response has `leech_lapses` when the answer is a lapse that reaches the leech threshold (or a multiple of it), `homophone: true` on a wrong `voice_to_zh` answer with the same pinyin and tones as the word, `ambiguous: true` and `ambiguous_meaning` (the shared translation) on a wrong `transl_to_zh` answer that is another word with the same meaning, and `needs_review: true` when the word is flagged for review |
| `GET` | `/api/quiz/match-game` | Get a round of words for the match mini-game, picked at random among the user's enabled game modes (mismatch / newest / hardest / last-mistakes) that currently have enough eligible words; empty `words` if none qualify. Mismatch tiles may be words or hanzi components; other modes are word-only |
| `POST` | `/api/quiz/match-answer` | Submit a match-game result — `{zh_word_id, correct}` for a word tile, or `{kind: "component", character, correct}` for a component tile — updates SM-2 or component progress |
| `POST` | `/api/quiz/accept-correct` | Accept a wrong answer as correct (typo), restoring pre-answer SM-2 progress. Send the same `drill`, `pinyin_seen`, `audio_played` and `play_pressed` values as with the answer, so the server removes the error from the same answer buckets |
| `GET` | `/api/quiz/langs` | List the distinct translation languages available |
| `POST` | `/api/quiz/skip` | Skip a word (defer due date by `days`, default 7). With `"reveal": true`, returns `200` with the word's `zh_text`, `pinyin` and `translations` instead of `204`. `/api/component/skip` (returns `correct_answers`) and `/api/hmm-quiz/skip` (returns `correct_answer`) accept `reveal` too. |
| `POST` | `/api/quiz/acknowledge` | Mark a new word as introduced (ready for quizzing) |
| `POST` | `/api/quiz/acknowledge-random` | Acknowledge a random subset of new words as immediately due, capped at the caller's remaining daily new-word allowance (`max_new_words_per_day` minus words already introduced today) — never bypasses the daily new-word cap |
| `POST` | `/api/quiz/advance` | Advance due dates / move past the current card |
| `POST` | `/api/quiz/difficult` | Flag the user's hardest words for a focused drill (`count` body field — about half by lowest accuracy, half by lowest ease factor); returns `{flagged}` |
| `POST` | `/api/quiz/difficult/clear` | End the difficult-words drill by clearing all drill flags |
| `GET` | `/api/quiz/wrong-today` | List the zh words answered wrong today (`?langs=en,de` for the translations): `{"words":[{word_id, zh_text, pinyin, translations}]}` |
| `POST` | `/api/quiz/wrong-today/retrain` | Make the words answered wrong today due now and remove them from the list until their next wrong answer. Returns `{"count": n}` |
| `GET` | `/api/quiz/stats` | Get due-today and total card counts (`tags` query param); includes `today_attempts`/`today_mistakes` (today's answers and wrong answers over words, components and mnemonic scenes), `difficult_remaining` (flagged drill words still to answer) and `words_improved_today` (words that moved into a better proficiency bucket today vs. the closest prior day's snapshot). When a baseline pauses new words, it also has `accuracy_pause_pct`/`accuracy_pause_min` or `lapsed_pause_count`/`lapsed_pause_max` |
| `GET` | `/api/quiz/daily-stats` | Get daily training stats history (attempts, mistakes, words known, new words, streak) |
| `GET` | `/api/quiz/word-stats` | Get per-word aggregate statistics: milestones, accuracy buckets, avg/median/P95, hardest & most-practiced words. The `tags` query param (comma-separated) restricts the accuracy-bucket breakdown to words matching the tags (OR in a tag group, AND between groups — see [Tag groups](features.md#vocabulary)) — hardest/most-practiced and other sections still cover all words |
| `GET` | `/api/quiz/due-date-distribution` | Get word counts grouped by due date for the next 30 days (`tags` query param) |
| `GET` | `/api/component/due-date-distribution` | Get seen hanzi component counts grouped by due date for the next 30 days |
| `GET` | `/api/components/coverage` | List every hanzi component appearing in the user's current zh vocabulary with the zh word IDs it covers, plus which of them already have a component_progress row (`trained_characters`) — used by the Settings page to preview how many components a candidate coverage-target percentage would select for training, how many are already in training, and how many of those would be out of scope at that target |
| `GET` | `/api/words` | List words (`q`, `page`, `per_page`, `sort`, `order`, `tags` query params; `due=today\|tomorrow\|known\|lapsed`) |
| `POST` | `/api/words` | Create a vocabulary entry |
| `POST` | `/api/words/upload-csv` | Bulk import from a CSV file (multipart: `file`, `tags`, `start_training_count`, optional `default_source` = `user` or `cedict`; optional `source` CSV column) |
| `GET` | `/api/import/source-tags` | Importable shared-library lists (HSK levels, topics) with their `word_count` |
| `GET` | `/api/import/preview` | Preview the words behind one or more lists (`tag` query param, repeatable). `match=all` counts only words that have every tag |
| `POST` | `/api/import` | Import (or update) a shared-library list as library references. Body: `tag`, `apply_tags`; optional `and_tags` (only words that also carry all of these tags), `import_mode` (`include` default, `review`, `known`) and `include_removed` (also add words you deleted) |
| `GET` | `/api/import/lists` | Lists you imported, each with `new` and `removed` counts |
| `GET` | `/api/words/{id}` | Get a single word with translations |
| `PUT` | `/api/words/{id}` | Update a word |
| `DELETE` | `/api/words/{id}` | Delete a word |
| `POST` | `/api/words/{id}/translations` | Add a single English translation to an existing word |
| `POST` | `/api/words/{id}/review` | Flag a word for review |
| `DELETE` | `/api/words/{id}/review` | Remove the review flag from a word |
| `POST` | `/api/words/{id}/reset` | Reset a word's SM-2 progress to unseen — removes it from every bucket and reintroduces it as new. Also erases its lapses and removes it from today's mistakes |
| `GET` | `/api/words/{id}/library-diff` | What a reset to the library changes on a library word: `remove` and `restore` (translations per language) and `pinyin` (`mine`, `library`; only when they differ). Empty for your own words |
| `POST` | `/api/words/{id}/reset-library` | Reset a library word to the library version (removes your added and deleted translations, takes the library pinyin; progress and tags stay). Returns the word, `404` if it is not a library word |
| `POST` | `/api/words/{id}/known` | Mark a word as known (`{"known": true}`) so it is never a quiz prompt, or return it to training (`{"known": false}`) |
| `GET` | `/api/audio/{id}` | Serve cached MP3 for a Chinese word (generated on demand) |
| `GET` | `/api/audio/component/{char}` | Serve cached MP3 for a single component character (generated on demand); files stored as `c_{hex}.mp3` |
| `GET` | `/api/hmm/breakdown` | Hanzi Movie Method breakdown (actor/location/room/props) for a word |
| `GET` | `/api/tags` | List all tag names (alphabetically) |
| `GET` | `/api/config` | Frontend feature flags (`deepl_enabled`, etc.) |
| `POST` | `/api/translate` | Translate text: free local CC-CEDICT/HanDeDict lookup first (zh→translation direction only), then DeepL fallback + generate pinyin (DeepL only available when `DEEPL_API_KEY` is set) |
| `GET` | `/api/github/config` | Whether in-app issue reporting is enabled (`{"enabled":bool}`) |
| `POST` | `/api/github/issues` | Create a GitHub issue from an in-app report (only available when `GITHUB_TOKEN` + `GITHUB_ISSUE_REPO` are set; rate-limited per user and per IP). Optional `client_error` text is added as a folded *Last error* block |
| `GET` | `/api/mismatches` | List all recorded confusion pairs (wrong answers that matched a different known word or hanzi component) |
| `GET` | `/api/hanzi/decompose` | Decompose Chinese characters into radicals and components (`chars` query param, max 20) |
| `GET` | `/api/hmm/actors` | List all HMM actor mappings (pinyin initial → person) |
| `PUT` | `/api/hmm/actors/{initial}` | Update actor name for an initial |
| `GET` | `/api/hmm/locations` | List all HMM location mappings (pinyin final → place) |
| `PUT` | `/api/hmm/locations/{final}` | Update location name for a final |
| `GET` | `/api/hmm/tone-rooms` | List all HMM tone room mappings (tone → room) |
| `PUT` | `/api/hmm/tone-rooms/{tone}` | Update room name for a tone |
| `GET` | `/api/hmm/props` | List all HMM prop mappings (radical → object) |
| `PUT` | `/api/hmm/props` | Create or update a prop mapping |
| `DELETE` | `/api/hmm/props/{radical}` | Delete a prop mapping |
| `GET` | `/api/words/{id}/hmm/context` | Get HMM scene context for a word (parsed pinyin, radicals, library lookups) |
| `PUT` | `/api/words/{id}/hmm` | Save mnemonic scene and auto-update library |
| `DELETE` | `/api/words/{id}/hmm` | Delete mnemonic scene |
| `POST` | `/api/components/{char}/review` | Flag a component for review |
| `DELETE` | `/api/components/{char}/review` | Remove the review flag from a component |
| `GET` | `/api/components/{char}/hmm-scene` | Get the saved mnemonic scene text for a component character |
| `PUT` | `/api/components/{char}/hmm-scene` | Save (or replace) a mnemonic scene text for a component character |
| `DELETE` | `/api/components/{char}/hmm-scene` | Delete the mnemonic scene for a component character |
| `GET` | `/api/pinyin-quiz/next` | Get the next pinyin sound to study (`tags` query param) |
| `POST` | `/api/pinyin-quiz/answer` | Submit a pinyin listening answer |
| `GET` | `/api/pinyin-quiz/stats` | Get pinyin due-today and total counts (`tags` query param) |
| `GET` | `/api/pinyin-quiz/audio/{filename}` | Serve a pinyin pronunciation MP3 file |
| `GET` | `/api/pinyin-quiz/tags` | List pinyin consonant group tags |
| `GET` | `/api/settings` | Get user settings (language prefs, quiz modes, masked API key status) |
| `PATCH` | `/api/settings` | Update language preferences and per-tier quiz mode configuration |
| `PATCH` | `/api/training-filters` | Persist training page filter state (mode, tier, langs, tags, mnemonics, components) server-side for cross-device sync |
| `POST` | `/api/sentences/generate` | Generate example sentences from known words with the LLM: `{"tags": [...], "count": 1-20, "focus": "all|hardest|mixups|frequent", "even": true}` (`focus` and `even` are optional; defaults `all` and `true`) → `{"focus_words": [...], "sentences": [{"zh_text", "pinyin", "translations"}]}`. Only sentences that consist of known words and that the user does not have yet. `400` when no known word matches the tags (for `mixups`: no mix-up of known words) or `focus` is not valid, `403` without plus role or own key, `503` without LLM |
| `PUT` | `/api/settings/api-keys` | Encrypt and store personal DeepL / LLM API keys |
| `PUT` | `/api/settings/ui-lang` | Store the app (UI) language: `{"ui_lang": "en" \| "de" \| "zh"}`. `GET /api/settings` returns it as `ui_lang` (empty until chosen) |
| `POST` | `/api/settings/backfill-subwords` | Create sub-words for every multi-character zh word already in active training (one-time backfill for words added before auto-subwords existed) |
| `GET` | `/api/admin/overview` | Cross-user usage insights: account/activity stats, signup and quiz-volume trends, guest activity, DeepL/LLM usage, page/feature usage (admin role only). Add `?exclude_seed=1` to leave the seed accounts (id 1, 2) out of every count. |
