# ADR-0002: SM-2 algorithm parameters are frozen

## Status

Accepted

## Context

The app uses the SM-2 spaced repetition algorithm to schedule vocabulary reviews. The two key quality parameters — `QualityCorrect = 4` and `QualityWrong = 0` — and the ease factor update formula were set during initial development.

There is ongoing temptation to tweak these values when users report cards feeling "too easy" or "too hard", or when comparing SM-2 to newer algorithms like FSRS.

## Decision

The SM-2 parameters (`QualityCorrect`, `QualityWrong`, the EF formula in `sm2.Update`) are frozen. They must not be adjusted speculatively or in response to anecdotal feedback.

Any change to these values requires:
1. A documented hypothesis about what the change improves
2. Evidence (data or literature) supporting the change
3. A new ADR superseding this one

Migrating to a different algorithm (e.g. FSRS) is out of scope until there is a concrete data-driven case to do so.

## Consequences

- Review scheduling remains stable and predictable across app versions
- Users' historical progress data stays valid — parameter changes would invalidate accumulated ease factors
- Developers must not treat these constants as tunable knobs during routine feature work
