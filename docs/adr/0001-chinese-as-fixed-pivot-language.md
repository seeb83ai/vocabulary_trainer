# ADR-0001: Chinese is the fixed pivot language

## Status

Accepted

## Context

The app was originally built as a Chinese–English vocabulary trainer. It has since grown to support German translations and is designed to support further native languages. A question arises: should the architecture be generalised so that any language pair is possible (e.g. Spanish–English, Japanese–German)?

Features like character breakdown (hanzi decomposition), pinyin listening training, and the Hanzi Movie Method mnemonic system are inherently Chinese-specific. They depend on Unicode CJK codepoints, pinyin romanisation, and stroke-order decomposition data — none of which apply to other writing systems.

## Decision

Chinese (`language = 'zh'`) is the fixed pivot language and will remain so. All quiz prompts are zh vocabulary entries. The native-language side (EN, DE, and future languages) may expand freely, but the Chinese side is not configurable.

Specifically:
- `GetNextCard` always filters `WHERE w.language = 'zh'`
- `word_id` in all quiz responses always refers to a zh entry
- Pinyin training, hanzi component breakdown, and HMM mnemonics are Chinese-specific and will not be abstracted for other languages
- Adding support for, say, Japanese would require a separate product, not a configuration flag

## Consequences

- The codebase remains focused and avoids premature generalisation
- Chinese-specific features (pinyin, components, mnemonics) can be developed without abstraction overhead
- A user who wants to learn Japanese cannot reuse this app — that is an accepted limitation
