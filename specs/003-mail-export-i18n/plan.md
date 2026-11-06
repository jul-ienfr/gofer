# Implementation Plan: Localize the Mail export experience

**Branch**: `003-mail-export-i18n` | **Date**: 2026-11-06 | **Spec**: [spec.md](./spec.md)

## Summary

Translate the Mail card and the Export mail flow through `internal/i18n`: card row, dialog title, account panel, folder form, job list, statuses, and handler errors. Keep the export job state machine, ZIP/mbox output, and endpoints unchanged.

## Technical Context

**Language/Version**: Go 1.27

**Primary Dependencies**: templ, templUI, Tailwind CSS, HTMX, existing `internal/i18n`

**Storage**: SQLite UI settings (`language`)

**Testing**: `go test ./...`, `go vet ./...`, templ generation

**Target Platform**: Self-hosted web application

**Constraints**: No new dependency, no schema change, no export behavior change

**Scale/Scope**: One card, one dialog, four export routes, two catalogs

## Constitution Check

- **User Data Sovereignty**: PASS — export data and selections are unchanged.
- **Safe by Default**: PASS — no credentials are added or exposed.
- **Provider Neutrality**: PASS — no provider-specific logic.
- **Test-Gated, Reversible Changes**: PASS — view, handler, parity, and French tests.
- **Boring, Reviewable Craft**: PASS — reuses existing helpers and components.

## Project Structure

```text
specs/003-mail-export-i18n/
├── spec.md
├── plan.md
└── tasks.md
```

```text
internal/i18n/messages_en.go
internal/i18n/messages_fr.go
internal/handler/mail_export_routes.go
internal/views/mail_export.templ
internal/views/export_import_tab.templ
internal/views/i18n_test.go
```

## Complexity Tracking

No constitution violations.
