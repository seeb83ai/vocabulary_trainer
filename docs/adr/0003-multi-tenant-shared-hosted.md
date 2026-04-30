# ADR-0003: Moving from single-user self-hosted to multi-tenant shared hosted

## Status

Accepted (in progress)

## Context

The app was originally built for a single user, self-hosted via Docker. Auth was a single `AUTH_USER`/`AUTH_PASSWORD` env var pair. All vocabulary and progress data was implicitly owned by one person.

The goal has shifted: the app is becoming a shared hosted instance where strangers can sign up with an email address and study independently. This requires proper user isolation, registration, and email verification.

## Decision

The app will operate as a shared hosted instance with multi-user accounts. Each user's vocabulary entries, translations, SM-2 progress, tags, pinyin progress, component progress, and mnemonic library are scoped to their `user_id`.

The legacy single-user auth env vars (`AUTH_USER`, `AUTH_PASSWORD`) are a stepping stone and will be phased out as the proper registration/email-verification flow matures.

## Consequences

- All DB queries that touch user-owned data must filter by `user_id`
- New features must be designed with data isolation in mind from the start
- The registration flow (POST `/api/register`, email verification via token) is the canonical way to create accounts
- Single-user deployment is still possible (one registered account) but is no longer the primary design target
