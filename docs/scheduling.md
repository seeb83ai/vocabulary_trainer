# Scheduling and quiz modes

How the app decides which card comes next: the daily new-word cap, the gates that pause new words, accuracy tiers, and the Progressive, Cycle and Random modes.

## Daily new-word cap

Each user sets their own daily new-word limit, in **Settings → Daily Learning**. The default is 5 words. The server-wide `MAX_NEW_WORDS` environment variable sets the default for new accounts only. Users can change it higher or lower in their own settings.

```bash
MAX_NEW_WORDS=5   # default for new accounts
```

A *new word* is one that has never appeared as a quiz card before. The app tracks this with a `first_seen_date` column in the database. Once you reach the daily cap, the app serves only cards you have already seen at least once. Reviews and retry cards are always available, regardless of the cap. The counter resets at midnight, server-local date.

The training page stats bar shows **New today: X / Y**, so you can see how many new words you have left for the day.

### New-word introduction order

When more than one never-seen word is eligible to be introduced, the app picks which one first using two rules, in order:

1. **Compound prerequisites.** If an unseen multi-character word is made up of substrings that are themselves existing, not-yet-introduced words in your vocabulary, the app introduces those component words first. For example, if your vocabulary contains 还, 可以, and 还可以, the app introduces 还 and 可以 before 还可以. Once every component word has been introduced, the compound is no longer held back.
2. **Word frequency.** Among words with the same priority under rule 1, the app prefers the more frequently used word, using a bundled frequency ranking that's imported automatically (see [Word-frequency import](data-import.md#word-frequency-import)). Words without a frequency ranking sort after ranked ones. Ties fall back to the existing due-date ordering.

Both rules only reorder *which* new word is shown next — they never change the [daily new-word cap](#daily-new-word-cap), [baseline gates](#baseline-gates), or [cooldown](#cooldown-between-new-words) behavior.

### Baseline gates

In **Settings → Daily Learning**, each user can enable optional gates. These gates pause new-word introductions when the review load is high. You enable each gate independently, with its own numeric threshold. All active gates must pass before the app shows a new word.

A "Max" gate blocks when the count reaches the threshold. For example, with a threshold of 10, the gate blocks at 10 words, so you can have at most 9.

Only the **Max New bucket words** gate uses your current tag filter. It counts only the words that match the filter. All other gates count all your words, also words outside the filter (for example, an untrained HSK level).

| Gate | Blocks new words when… |
|---|---|
| **Max due words at day start** | The number of review cards due when you first opened the app today is ≥ the threshold |
| **Max Struggling words** | Your current Struggling bucket count is ≥ the threshold |
| **Max Learning words** | Your current Learning bucket count is ≥ the threshold |
| **Max New bucket words** | Your current New bucket count (already-introduced words that haven't graduated past the initial learning phase yet) is ≥ the threshold |
| **Min. accuracy of the last 3 days** | Your accuracy over the 3 days before today (all answers of these days together) is below the threshold in %. Today counts too when it has at least 20 answers, so a bad session pauses new words on the same day. Days without answers are not counted. If there are no answers, the gate does not block. Default: off, 70%. When this gate blocks, the Train page shows a small "!" right of the progress bar. Tap it to read "New words paused: accuracy X% < Y%". |
| **Max words that failed their last review** | The number of words whose last review was a [lapse](#lapses-and-leech-words) is ≥ the threshold. A word leaves the count when you answer its next review correctly. Default: **on**, 10. When this gate blocks, the "!" reads "New words paused: N words failed their last review (limit M)". If the accuracy gate also blocks, the accuracy message shows. |

### Cooldown between new words

In **Settings → Daily Learning**, the **Cooldown between new words** field sets the minimum time between new-word introductions. The default is 1 minute. During the cooldown window, the app serves only review cards. Set the value to **0** to disable the cooldown and allow new words back-to-back.

The cooldown is automatically skipped while you have fewer than 20 introduced words — it exists to pace new-word introduction for an established vocabulary, and would otherwise just stall onboarding for a brand-new account.

### Lapses and leech words

A **lapse** is a failed review: the first answer of the day is wrong, the word is due today, and the word has left the New bucket. Only the main quiz counts. Wrong retries on the same day, the difficult-words drill, the match game, pinyin, components and mnemonics do not count. A correct first answer of the day ends the run of lapses. If you accept a wrong answer as a typo, the lapse is undone.

A **leech** is a word that fails its review several times in a row. In **Settings → Daily Learning**, **Warn after failed reviews in a row** sets the number (default 5, 0 = off). When a word reaches this number, the wrong-answer screen shows a box: "You failed this word N reviews in a row". The box has two buttons:

- **Keep training**: the box closes and the normal flow continues. The box shows again at each multiple of the number (10, 15 …).
- **Back to unseen**: the word goes back to your unseen words and its history is erased (the same as *Reset progress* in Vocabulary). It leaves today's session and today's mistakes. It goes to the end of the new-word queue.

In **Vocabulary**, the *Failed last review* chip lists the words whose last review was a lapse. Open a word and use *Reset progress* to put it back to your unseen words.

### Skip button for new words

By default, a **Skip** button appears on the new-word introduction screen. It lets you defer a word for 7 days. In **Settings → Daily Learning**, you can hide this button. When you hide it, you cannot skip new words, and you must review them.

### Session extension (avoid immediate repetition)

Near the end of a session, if the only review card left due today is the one you just answered, the app serves a not-yet-due word instead of repeating it right away. The **Due today** counter on the training page accounts for this, so the number you see always matches what the app will actually ask. In **Settings → Daily Learning**, the **Add extra words at the end of a session to avoid repetition** toggle controls this behavior. It is on by default. Turn it off to receive only genuinely due-today words, even if that means the app occasionally repeats one right away.

## Progressive mode

The **Progressive** quiz mode introduces new words gently and increases difficulty gradually, based on your accuracy (correct answers divided by total attempts). By default, the app behaves as follows:

| Condition | What happens |
|---|---|
| Brand new word (`total_attempts = 0`) | **Introduction** — shows Chinese, pinyin, and all translations. No quiz. Choose "Got it" to start learning or "Skip" to defer 7 days. Pinyin covers the full Chinese text, including any bracketed annotation (e.g. `过（动词）`). If your settings require typing the Chinese and/or translation to confirm, a bracketed annotation is optional to type — mirroring the parenthesized-segment rule used everywhere else in answer checking. |
| **Learning phase** (`learning_new_word = true`) | The word is in the **New** bucket. Short retry intervals (minutes, not days) let you drill it in one session. Three correct answers in a row graduate the word. A wrong answer resets the streak. |
| `total_attempts < 3` | **Translation → Chinese** — not enough data yet, so the app uses the easiest direction |
| Accuracy < 50% | **Translation → Chinese** — you are still struggling |
| Accuracy < 70% **or** `total_attempts < 10` | **Chinese + Pinyin → Translation** — you are making progress |
| Accuracy < 85% (and `total_attempts ≥ 10`) | **Chinese → Translation** — reliable; you see Chinese only |
| Accuracy ≥ 85% and `total_attempts ≥ 10` | **Random** — any of the three quiz directions |

You can customize the quiz format for each tier, and for each step of the new-word learning phase, in **Settings → Training Mode**. The available formats are: *Translation → Chinese*, *Chinese → Translation*, *Chinese + Pinyin → Translation*, *Translation → Chinese (pinyin hint)*, and *Random*.

A *Translation → Chinese* question never shows a dictionary example sentence (`Bsp.:`) or a measure-word annotation (`CL:`/`ZEW:`) as the prompt word. The app uses the first real translation. If a word has only such entries in the selected languages, the card uses *Chinese → Translation*.

**Learning phase ("New" bucket):**

When you acknowledge a new word by clicking "Got it," it enters the learning phase. During this phase:

- The app uses short intervals (1-5 minutes) instead of day-scale SM-2 intervals.
- You need **3 consecutive correct answers** to graduate.
- A wrong answer resets the streak counter to 0.
- On graduation, the app resets SM-2 progress to a clean baseline. Accuracy starts at 100%, `total_attempts` is set to 3, and the word moves to the regular review queue with a 1-day interval.

The training page and vocabulary list show a **New** tier badge for words still in the learning phase. You can filter by the "New" bucket to drill only recently introduced words.

**Accuracy tiers:**

Tier assignment uses **effective accuracy**, calculated as `(total_correct + streak_bonus) / total_attempts`. The `streak_bonus` speeds up recovery from initial mistakes. See "Streak bonus" below.

| Tier | Criteria |
|---|---|
| **New** | `learning_new_word = true` (still in learning phase) |
| **Struggling** | `< 3 attempts` or `effective accuracy < 50%` |
| **Learning** | `≥ 3 attempts` and `50% ≤ effective accuracy < 70%` |
| **Practicing** | `≥ 10 attempts` and `70% ≤ effective accuracy < 85%` |
| **Mastered** | `≥ 10 attempts` and `effective accuracy ≥ 85%` |

**Streak bonus:**

When you answer a word correctly several times in a row, the app grants a `streak_bonus`. This bonus boosts effective accuracy to match the bucket for your current streak length.

| Consecutive correct (streak) | Target bucket | Min effective accuracy |
|---|---|---|
| < 3 | _(no boost)_ | — |
| 3-5 | Learning | 50% |
| 6-8 | Practicing | 70% |
| ≥ 9 | Mastered | 85% |

The app calculates the bonus as the minimum value needed to reach the target accuracy threshold. The bonus never decreases. Once you earn it, it persists even if the streak later breaks. However, wrong answers dilute the bonus over time, because `total_attempts` grows while `streak_bonus` stays fixed. The training page shows effective accuracy with a "(+N streak bonus)" indicator when a bonus is active.

**Skip vs Got it:**

- **Got it** marks the word as introduced, starts the learning phase, and makes the word available for quizzing right away (EN → ZH). This counts toward the daily new-word cap.
- **Skip** defers the word by 7 days. This does *not* count as seen. The word remains "new," and the app shows it as an introduction again when it comes due.

**Skip for Today:** Below the Submit button on the training card, a secondary **Skip for Today** button defers the current card (word, HMM mnemonic, or component) by 1 day, without recording an attempt. Use this to clear a stuck card from today's queue and try again tomorrow. If you turn on **Settings → Training → Show the answer when I skip** (off by default), a *Skipped* screen shows the answer of the card (the word with pinyin and translations, the component definitions, or the mnemonic name) before the next card. The skip still counts as no attempt.

If you answer the same card wrong 3 times in a row in this session, the wrong-answer screen also shows **Skip for today** below the Next button. It defers the card by 1 day and goes to the next card. The wrong answers stay recorded. A correct answer (or *Accept as correct*) sets the count back to 0. The count is per card and starts again when you reload the page.

## Cycle mode

The **Cycle** quiz mode rotates through a fixed sequence of quiz directions. By default, the position advances on every attempt, correct or wrong, so the counter is `total_attempts`. In Settings → Cycle Mode, you can switch to **advance on success only**. This mode uses `total_correct` as the counter instead, so the step stays the same until you answer correctly. A stricter **advance only if known** option uses a first-try-correct counter instead: the step only advances once you get the answer right on your *first* attempt for that encounter — a wrong guess before the correct answer keeps the step the same, even though the word is eventually marked correct.

Default sequence: **Chinese + Pinyin → Translation → Chinese → Translation → Chinese → Translation**

| `total_attempts` | Position | Direction |
|---|---|---|
| 1 (after "Got it") | 0 | Chinese + Pinyin → Translation |
| 2 | 1 | Translation → Chinese |
| 3 | 2 | Chinese → Translation |
| 4 | 0 (wraps) | Chinese + Pinyin → Translation |

This sequence, like every other training-mode setting, only takes effect once a word has **graduated** out of the new-word introduction phase. While `learning_new_word = true`, the overall training mode you have selected (Cycle, Progressive, Random, or a fixed direction) never matters — the quiz format always comes from the **New word phase** steps configured in Settings → Training Mode instead (see "Progressive mode" above).

You can configure the sequence (2 to 7 steps) in **Settings → Cycle Mode**. The available directions are: *Translation → Chinese*, *Chinese → Translation*, *Chinese (no sound) → Translation*, *Chinese + Pinyin → Translation*, *Voice → Translation*, *Voice → Chinese*, and *Translation → Chinese (pinyin hint)*. The same settings panel offers three mutually exclusive advance options: **Advance on every attempt** (default, counter is `total_attempts`), **Advance only on success** (counter is `total_correct`), and **Advance only if known** (counter is a first-try-correct count that only increments when an answer is correct with no wrong guess earlier in that encounter).

Under **advance only on success** or **advance only if known**, a wrong answer holds the step at exactly the format that was just shown — even if that answer also moves the word into a different accuracy tier (which would otherwise re-filter the step against a different "Random / Cycle mode by bucket" range, see below, and could pick a different format for the same step). The pinned format is cleared as soon as the encounter is resolved by a correct answer.

## Random / Cycle mode by bucket

**Random** mode and **Cycle** mode both pick from 6 quiz formats: *Translation → Chinese*, *Chinese → Translation*, *Chinese (no sound) → Translation*, *Chinese + Pinyin → Translation*, *Voice → Translation*, and *Voice → Chinese*. In **Settings → Random / Cycle Mode by Bucket**, you can restrict which of these formats is eligible for each of the 5 accuracy tiers (New, Struggling, Learning, Practicing, Mastered — the same buckets used elsewhere in the app). For each format, choose an inclusive "from" and "to" bucket range, or turn the format off entirely.

- **Random mode** picks uniformly at random among the formats eligible for the word's current bucket.
- **Cycle mode** filters your configured step sequence down to the formats eligible for the word's current bucket before picking a step. If none of your configured steps are eligible for that bucket, the app falls back to picking from the full bucket-eligible format list instead. This filtering only applies to a graduated word — a word still in the new-word introduction phase ignores it entirely (see "Cycle mode" above).

On save, the app checks that every one of the 5 buckets has at least one eligible format across all 6 settings combined. If a bucket would be left with no eligible format, the save is rejected and nothing changes.

The default ladder increases difficulty (fewer hints) for higher buckets:

| Format | Default range |
|---|---|
| Translation → Chinese | New – Learning |
| Chinese + Pinyin → Translation | New – Practicing |
| Chinese → Translation | Struggling – Mastered |
| Chinese (no sound) → Translation | Learning – Mastered |
| Voice → Translation | Practicing – Mastered |
| Voice → Chinese | Practicing – Mastered |
