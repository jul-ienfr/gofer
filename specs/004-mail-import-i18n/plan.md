# Implementation Plan: Localize the Import mail experience

**Branch**: `004-mail-import-i18n` | **Date**: 2026-11-06 | **Spec**: [spec.md](./spec.md)

## Summary

Translate the Import mail upload, review, destination selectors, job list, account problems, and handler errors through `internal/i18n`. Keep mbox/eml/ZIP parsing, deduplication, and job behavior unchanged.

## Technical Context

**Language/Version**: Go 1.27

**Primary Dependencies**: templ, templUI, Tailwind CSS, HTMX, existing `internal/i18n`

**Storage**: SQLite UI settings (`language`)

**Testing**: `go test ./...`, `go vet ./...`, templ generation

**Constraints**: No new dependency, no schema change, no import behavior change

**Scale/Scope**: One dialog, upload/review/jobs views, four routes, two catalogs

## Constitution Check

- **User Data Sovereignty**: PASS — imported data and choices unchanged.
- **Safe by Default**: PASS — no credentials added or exposed.
- **Provider Neutrality**: PASS — no provider-specific logic.
- **Test-Gated, Reversible Changes**: PASS — view, handler, parity, and French tests.
- **Boring, Reviewable Craft**: PASS — reuses existing helpers.

## Project Structure

```text
specs/004-mail-import-i18n/
├── spec.md
├── plan.md
└── tasks.md
```

```text
internal/i18n/messages_en.go
internal/i18n/messages_fr.go
internal/handler/mail_import.go
internal/handler/mail_import_routes.go
internal/views/mail_import.templ
internal/views/i18n_test.go
```

## Complexity Tracking

No constitution violations.
