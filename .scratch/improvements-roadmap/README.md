# Improvements roadmap — issue batches

Derived from `PLAN_IMPROVEMENTS.md`. Each issue is one batch = one PR, sized to be
manually testable and reviewable. Implemented serially, top to bottom; merge each before
the next.

| # | Batch | Plan items | Blocked by |
|---|-------|-----------|------------|
| 01 | HTTP server hardening (timeouts, shutdown, body limits) | 1.1, 1.2 | — |
| 02 | Auth & client-IP hardening | 1.6, 1.9, 1.7 | — |
| 03 | Error-handling & logging hygiene | 1.3, 1.4, 1.5, 1.8 | — |
| 04 | Quiz request-state perf | 1.10, 1.11 | — |
| 05 | CSV DoS + reflected errors | 2.1, 2.2, 2.3 | — |
| 06 | CI quality gates | 3.1, 3.2, 3.3 | — |
| 07 | Backup + migration tests | 4.1, 4.2 | — |
| 08 | Schema cleanups | 4.3, 4.4 | — |
| 09 | Per-user component dictionary | 4.5 | — |
| 10 | Trade-off ADRs | 4.6 + single-host | — |
| 11 | Quiz-answer seams | 5.1, 5.2, 5.3 | — |
| 12 | Tier single-source | 5.4 | — |
| 13 | Training-init + card scheduler | 5.5, 5.6 | 11 |
| 14 | Split Store into sub-stores | 5.7 | 13 |
| 15 | Test coverage | 6.1, 6.2, 6.3, 6.4 | — |

Only 13→11 and 14→13 are hard dependencies; everything else can be reordered freely.
All batches are AFK (the one HITL decision — the Store-split grouping — was resolved during
planning).
