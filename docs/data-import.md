# Data import tools

Command-line tools that fill the database: word lists, dictionaries, frequency data and audio. They all call `db.Migrate()`, so they also work against a new database. Each has a `make` target and a `-dry-run` flag.

To set up the shared library from scratch, run them in this order: `import-cedict` / `import-handedict` → `import-hsk` (both versions) → `classify-topics` (optional) → `import-topics`. `import-hanzi` and `import-pinyin` are independent.

## Bulk import

You can import vocabulary from a plain-text file, in this format (3 lines per entry, blank lines ignored):

```
pinyin / 汉字
translation(s), comma-separated
rating string (ignored)
```

```bash
# Default: reads voc.txt, writes to data/vocab.db
make import

# Custom paths
make import FILE=my_vocab.txt DB=data/vocab.db

# Preview without writing
go run ./service/cmd/import -db data/vocab.db -file voc.txt -dry-run
```

Duplicate detection prevents the app from re-inserting entries where both the Chinese text or pinyin and the English translation already exist.

## HSK vocabulary import

This tool adds an HSK word list to the shared library user (id=1). Users then import the list from the library on the training page or in **Vocabulary → Import**.

The source is `complete.json` from [drkameleon/complete-hsk-vocabulary](https://github.com/drkameleon/complete-hsk-vocabulary) (MIT licence). The tool downloads the file, or reads a local copy with `-file`.

| `-version` | List | Tags |
|---|---|---|
| `3` (default) | HSK 3.0, 2025 syllabus | `hsk3-1` … `hsk3-6`, `hsk3-7` (levels 7–9) |
| `2` | HSK 2.0 | `hsk2-1` … `hsk2-6` |

The tool adds the Chinese word, the pinyin and the tag. At the end, it fills the library translations of new words from CC-CEDICT/HanDeDict (see `make import-cedict` and `make refresh-library`). If a word is already in the library, the tool does not add it again. It only adds the tag. The tool marks each tag as importable and gives it a description, for example "HSK 3.0 – Level 1". You can run the tool again safely.

```bash
# Import HSK 3.0 (2025 syllabus)
make import-hsk

# Import HSK 2.0
make import-hsk VERSION=2

# Custom DB path
make import-hsk DB=/path/to/vocab.db

# Use a local copy of complete.json and preview without writing
go run ./service/cmd/import-hsk -file complete.json -dry-run
```

Flags:

| Flag | Default | Description |
|---|---|---|
| `-db` | `data/vocab.db` | Path to SQLite database |
| `-version` | `3` | HSK version: `3` (HSK 3.0, 2025 syllabus) or `2` (HSK 2.0) |
| `-file` | – | Read `complete.json` from this file instead of downloading it |
| `-url` | GitHub raw URL | Download URL of `complete.json` |
| `-dry-run` | false | Show what would change without writing |

Older versions of the app used the tags `hsk1` … `hsk6` for HSK 2.0. A migration renames them to `hsk2-1` … `hsk2-6` (and `s_hsk1` … to `s_hsk2-1` …) for all users, and updates saved training filters.

Older versions also marked sentences with an `s_` prefix (`s_hsk2-1`). A migration renames every tag `s_X` to `X_s` (`s_hsk2-1` to `hsk2-1_s`) for all users, and updates saved training filters. If `X_s` already exists, the migration merges the two tags. A tag with the `s_` prefix is no longer a sentence.

## Word-frequency import

A standalone `word_frequency_lang(word, lang, rank)` reference table (rank 1 = most frequent) serves two features: the zh list orders [new-word introduction](scheduling.md#new-word-introduction-order), and the en/de lists power [translation ranking](features.md#translations-on-cards) (scoring each CEDICT/HanDeDict-derived translation gloss by its rarest word, so training can hide the least important ones). It does not create or modify any vocabulary words, and a ranked word that isn't currently used is simply unused. (An older `word_frequency(word, rank)` table from before the en/de lists existed is still present for backward compatibility but is no longer read.)

Three 8,000-entry lists derived from [hermitdave/FrequencyWords](https://github.com/hermitdave/FrequencyWords) (MIT license) — `content/2018/zh_cn/zh_cn_50k.txt`, `content/2018/en/en_50k.txt`, and `content/2018/de/de_50k.txt` (OpenSubtitles-based corpora) — filtered to entries made up solely of CJK ideographs (zh) or alphabetic characters (en/de), deduplicated, and truncated to the top 8,000 by frequency — are bundled with the app and **imported automatically by schema migrations on startup**. No manual step is required; this is unlike the optional `import-hanzi`/`import-cedict` tools, since without it the frequency-ordering and translation-ranking features above would silently do nothing.

For `en` and `de`, `make import-frequency` also rebuilds the `gloss_rank` table, because the gloss ranks come from these lists.

A `make import-frequency` CLI tool remains available for importing an alternative or updated frequency list, or to force a re-import of a bundled one:

```bash
# Re-import the bundled zh list
make import-frequency

# Custom DB path, data file, or language (zh/en/de)
make import-frequency DB=/path/to/vocab.db FILE=/path/to/frequency_data.txt LANG=en

# Preview without writing
go run ./service/cmd/import-frequency -dry-run
```

Flags:

| Flag | Default | Description |
|---|---|---|
| `-db` | `data/vocab.db` | Path to SQLite database |
| `-file` | `frequency_data.txt` | Path to a `word<TAB>rank` list (lines starting with `#` are treated as comments) |
| `-lang` | `zh` | Language the list is for (`zh`, `en`, or `de`) |
| `-dry-run` | false | Parse and validate without writing |

Re-running the import updates the rank of any word already present for that language rather than duplicating it, so it's safe to re-run after refreshing the data file.

## Character decomposition import (makemeahanzi)

This tool imports character decomposition data from the [makemeahanzi](https://github.com/skishore/makemeahanzi) project's `dictionary.txt` file. It enables the "Character breakdown" feature on the training screen.

1. Download `dictionary.txt` from the makemeahanzi repository into the project root.
2. Run the import:

```bash
make import-hanzi                          # uses dictionary.txt in project root
make import-hanzi FILE=/path/to/dictionary.txt  # custom path
```

Or directly:

```bash
cd service && go run ./cmd/import-hanzi -db ../data/vocab.db -file ../dictionary.txt
```

| Flag | Default | Description |
|---|---|---|
| `-db` | `data/vocab.db` | Path to SQLite database |
| `-file` | *(required)* | Path to makemeahanzi `dictionary.txt` |
| `-dry-run` | false | Parse and validate without writing |

## Bilingual dictionary import (CC-CEDICT / HanDeDict)

This tool imports a CEDICT-format dictionary — [CC-CEDICT](https://www.mdbg.net/chinese/dictionary?page=cedict) (Chinese→English) or [HanDeDict](https://handedict.zydeo.net/en/) (Chinese→German, distributed in the same line format) — for two features: segmenting multi-character vocabulary words into their constituent sub-words (single-character parts become trainable components automatically; multi-character parts are auto-created as new, tagged vocabulary words — see [Sub-word detection](features.md#characters-and-components)), and a free, ungated dictionary lookup tried before DeepL in the word add/edit "Translate" flow.

1. Download `cedict_ts.u8` from [MDBG](https://www.mdbg.net/chinese/dictionary?page=cedict) (CC BY-SA 4.0) and/or a HanDeDict data file from [handedict.zydeo.net](https://handedict.zydeo.net/en/) (CC BY-SA 2.0 DE) into the project root.
2. Run the import for each dictionary you have:

```bash
make import-cedict                              # EN, uses cedict_ts.u8 in project root
make import-cedict FILE=/path/to/cedict_ts.u8    # custom path
make import-handedict                            # DE, uses handedict.u8 in project root
make import-handedict FILE=/path/to/handedict.u8 # custom path
```

After each run, the tool rebuilds the `gloss_rank` table. This table stores the frequency rank of every dictionary gloss, so adding a dictionary translation does not look up each word of the gloss again. Then it refreshes the shared library (see "Library refresh" below), so users see the new translations at once.

A run **replaces** the stored version of that dictionary: entries that upstream removed go away. Entries kept from the old curated library (`source = 'user'`) and the other language stay. A translation a user deleted stays deleted, also when a later version removes and then brings back that sense. If the file has less than half the entries of the stored dictionary, the tool stops, because a partial file would remove most of the dictionary. To add a partial file, use `APPEND=1` (`-append`); to replace anyway, use `FORCE=1` (`-force`):

```bash
make import-cedict FILE=/path/to/extra.u8 APPEND=1
```

Or directly:

```bash
cd service && go run ./cmd/import-cedict -db ../data/vocab.db -file ../cedict_ts.u8 -lang en
cd service && go run ./cmd/import-cedict -db ../data/vocab.db -file ../handedict.u8 -lang de
```

| Flag | Default | Description |
|---|---|---|
| `-db` | `data/vocab.db` | Path to SQLite database |
| `-file` | *(required)* | Path to a CEDICT-format dictionary file |
| `-lang` | *(required)* | `en` (CC-CEDICT) or `de` (HanDeDict) |
| `-dry-run` | false | Parse and validate without writing |
| `-append` | false | Add the entries instead of replacing the stored dictionary |
| `-force` | false | Replace even when the file has less than half the stored entries |

Like `import-hanzi`, this is a manual, one-time (or re-run-on-update) operational step against `data/vocab.db` — it is not run automatically at startup.

## Library refresh (refresh-library)

The shared library (user 1) keeps its own copy of the dictionary translations of its words. Users who imported a list see these translations directly, they do not get a copy (see [ADR-0005](adr/0005-shared-library-with-per-user-references.md)). You do not need to run the refresh by hand: the server fills the library on the first start after the upgrade (see [Upgrade to library references](self-hosting.md#upgrade-to-library-references-one-time-conversion)), and `import-cedict`, `import-hsk` and `import-topics` run the refresh at the end. To run it alone (for example after changing `cedict_entries` by hand):

```bash
make refresh-library
make refresh-library DB=/path/to/vocab.db
```

The tool splits each dictionary definition into senses, as the import does, and compares them with the library:

- A new sense is added. An unchanged sense keeps its row, so the users' changes stay valid.
- A sense the dictionary dropped is removed from the word.
- A word whose translations changed is marked. Users who changed that word themselves are asked on the import screen whether to keep their version.
- A word that is no longer in the dictionary keeps its last translations and is marked as removed. It is never deleted.

You can run the tool again safely. A translation that a user deleted stays deleted, also when the dictionary drops the sense and a later version brings it back.

## Topic word lists (classify-topics)

A one-off tool that sorts vocabulary into topic word lists (travel, food, body, health, …, 28 topics in total) with Claude. The app does not read the CSV files. `import-topics` (see below) loads them into the shared library as importable word lists.

The candidates are every library word tagged with an HSK level (`hsk2-N` or `hsk3-N`) and every word in the top `-freq` of the zh frequency list that has a CC-CEDICT entry, so run `import-hsk` (both versions) and `import-cedict` first. Claude gives each word 0–2 topics. The tool appends each word to `<out>/<topic>.csv` (columns `zh,en`, where `en` is the first CC-CEDICT gloss). Words that fit no topic go to `<out>/_none.csv`. The tool skips every word that is already in a CSV file, so after an error or a Ctrl-C you can run it again and it continues where it stopped. On Ctrl-C, the tool starts no new requests but waits for the running requests and writes their results. A second Ctrl-C quits at once, and the running requests are done again on the next run.

```bash
# See how many words would be classified (no API calls)
make classify-topics DRY=1

ANTHROPIC_API_KEY=... make classify-topics

# Or use your Claude subscription through a logged-in Claude Code install (no API key)
make classify-topics CLI=claude

# `make release` ships a prebuilt binary and copies data/topics/ (if present) to data/topics/ on the server.
# To run the classification on the server instead:
ANTHROPIC_API_KEY=... ./classify-topics -db data/vocab.db -out data/topics
```

| Flag | Default | Description |
|---|---|---|
| `-db` | `data/vocab.db` | Path to SQLite database |
| `-out` | `data/topics` | Directory for the `<topic>.csv` files |
| `-freq` | `8000` | Also classify non-HSK words up to this zh frequency rank |
| `-chunk` | `100` | Words per Claude request |
| `-workers` | `4` | Claude requests to run at the same time |
| `-model` | `claude-opus-5` | Claude model ID |
| `-effort` | `low` | Claude effort level |
| `-claude-cli` | (empty) | Path or name of a Claude Code binary (for example `claude`). When set, each request runs through `claude -p` with your Claude Code login (subscription) instead of `ANTHROPIC_API_KEY`. Tools are turned off and the tool's system prompt replaces Claude Code's own. Subscription usage limits apply; when you reach a limit, the tool stops, and you can run it again later to continue. |
| `-dry-run` | false | Print the number of words to classify, without calling Claude |

With HSK 2.0 + 3.0 and `-freq 8000`, there are about 12,600 candidate words (about 126 requests).

## Topic word list import (import-topics)

This tool loads the `data/topics/<topic>.csv` lists from `classify-topics` into the shared library (user 1). It tags each word with `topic-<topic>` (for example `topic-travel`) and marks each tag as importable. Users then see the topic lists next to the HSK lists in onboarding ("Let me choose word lists myself") and in Vocabulary → Import. When a user imports a list, translations come from CC-CEDICT/HanDeDict, as with the HSK lists. Words without a dictionary translation are skipped. `_none.csv` is not imported.

If a word is not in the library yet (the non-HSK frequency words), the tool adds it with its CC-CEDICT pinyin. You can run the tool again safely: it only adds missing tags and words. Run `import-cedict` first, so that the new words get pinyin.

```bash
make import-topics DRY=1   # preview
make import-topics

# On the server, after `make release` (which also copies data/topics/):
./import-topics -db data/vocab.db -dir data/topics
```

| Flag | Default | Description |
|---|---|---|
| `-db` | `data/vocab.db` | Path to SQLite database |
| `-dir` | `data/topics` | Directory with the `<topic>.csv` files |
| `-dry-run` | false | Show what would change without writing |

## Pinyin audio import

This tool imports pinyin pronunciation audio files from the public-domain [mp3-chinese-pinyin-sound](https://github.com/davinfifield/mp3-chinese-pinyin-sound) collection. It enables the `/pinyin` listening training page.

1. Clone the audio repository:
```bash
git clone https://github.com/davinfifield/mp3-chinese-pinyin-sound.git
```

2. Import the audio files:
```bash
make import-pinyin SOURCE=mp3-chinese-pinyin-sound/mp3
```

Or directly:
```bash
cd service && go run ./cmd/import-pinyin -source ../mp3-chinese-pinyin-sound/mp3
```

| Flag | Default | Description |
|---|---|---|
| `-db` | `data/vocab.db` | Path to SQLite database |
| `-source` | `mp3` | Directory containing pinyin MP3 files (e.g. `ba1.mp3`) |
| `-audio-dir` | `data/pinyin-audio` | Destination directory for audio files |
| `-dry-run` | false | Parse files without writing to DB or copying |

The app stores audio files in `PINYIN_AUDIO_DIR` (default: `data/pinyin-audio/`). Set this environment variable to override the default location.
