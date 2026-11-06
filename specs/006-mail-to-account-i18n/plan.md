# Implementation Plan: Localize the Move to another account dialog

**Branch**: `006-mail-to-account-i18n` | **Date**: 2026-11-06 | **Spec**: [spec.md](./spec.md)

## Summary

Translate the message-list Move to another account dialog and its handler errors through `internal/i18n`. Keep move/copy behavior unchanged.

## Technical Context

**Language/Version**: Go 1.27

**Primary Dependencies**: templ, templUI, Tailwind CSS, HTMX, existing `internal/i18n`

**Storage**: SQLite UI settings (`language`)

**Testing**: `go test ./...`, `go vet ./...`, templ generation

**Constraints**: No new dependency, no schema change, no behavior change

**Scale/Scope**: One dialog, two routes, two catalogs

## Constitution Check

- **User Data Sovereignty**: PASS — message moves and choices unchanged.
- **Safe by Default**: PASS — no credentials added or exposed.
- **Provider Neutrality**: PASS — no provider-specific logic.
- **Test-Gated, Reversible Changes**: PASS — view, parity, and French tests.
- **Boring, Reviewable Craft**: PASS — reuses existing helpers.

## Project Structure

```text
specs/006-mail-to-account-i18n/
├── spec.md
├── plan.md
└── tasks.md
```

```text
internal/i18n/messages_en.go
internal/i18n/messages_fr.go
internal/handler/mail_transfer_routes.go
internal/views/mail_to_account.templ
internal/views/i18n_test.go
```

## Complexity Tracking

No constitution violations.
