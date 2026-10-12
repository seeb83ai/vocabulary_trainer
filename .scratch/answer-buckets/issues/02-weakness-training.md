# Train words by the user's weak answer buckets

Status: needs-triage

## Parent

Answer-bucket tracking (`answer_stats_daily`, `answer_stats_monthly`). Best done after
`01-bucket-stats-view.md`, so the user can see the weakness before they train it.

## What to build

A training option that picks words and modes from the user's weak answer buckets. For
example: the user is often wrong on voice-only cards, so the session shows more
`voice_to_transl` cards for the words that failed in that bucket.

## Open questions

- Use the error rate per word and bucket, or the bucket rate over all words?
- Time window: last 30 days (daily table), or all time (monthly table)?
- Does a word reset change anything? Today a reset keeps the stats. The training can
  ignore data from before the reset if needed.
- How does it fit next to the difficult-words drill and the progressive mode?
- SM-2 parameters are frozen (ADR-0002). This feature must only choose cards and modes,
  not change the scheduling.
