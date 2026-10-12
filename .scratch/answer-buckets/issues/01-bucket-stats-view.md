# Show error rates per answer bucket

Status: needs-triage

## Parent

Answer-bucket tracking (`answer_stats_daily`, `answer_stats_monthly`, see README
"Answer buckets" and the CONTEXT.md term *Answer bucket*).

## What to build

Show the user how often they are wrong in each answer bucket (`zh_only`, `pinyin`,
`voice`, `voice_only`, `to_zh`). The user must see where they are weak, for example
"voice only: 40 % wrong, hanzi only: 12 % wrong".

## Rules from the tracking design

- Last 30 days by day: read only `answer_stats_daily`.
- Long term by month: read only `answer_stats_monthly`.
- Never add the two tables together. The last 30 days and the current month overlap.
- An answer with pinyin and sound counts in two buckets, so the bucket totals are not
  equal to the total number of answers.
- Filter daily rows with `day >= date('now', '-30 days')`. Old rows of inactive users
  can still be in the table.

## Open questions

- Where does the view go: the Stats page, or a new tab?
- Chart type and time range control.
- Minimum number of attempts before a rate is shown (small numbers give noisy rates).
- Do we show `to_zh` next to the ZH → translation buckets, or apart?

## Acceptance criteria

- [ ] New read API with handler test, registered in `main.go` and `newRouter()`
- [ ] Store query with unit test (in-memory SQLite)
- [ ] E2E test and PR screenshots
