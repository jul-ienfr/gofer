# Implementation Plan: Localize the Thunderbird import experience

**Branch**: `001-export-import-i18n` | **Date**: 2026-11-06 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/001-export-import-i18n/spec.md`

## Summary

Route every user-visible string in the Thunderbird import card, dialog, preview, result, warning, and error through the existing `internal/i18n` catalogs. Resolve the owner's locale once per request, keep the import behavior and HTTP contract unchanged, and add catalog-parity plus French rendering coverage.

## Technical Context

**Language/Version**: Go 1.27

**Primary Dependencies**: templ, templUI, Tailwind CSS, HTMX, existing `internal/i18n` package

**Storage**: SQLite UI settings (`language`) through the existing per-user store

**Testing**: `go test ./...`, `go vet ./...`, templ generation

**Target Platform**: Self-hosted web application (single binary)

**Project Type**: Web service with server-rendered views

**Performance Goals**: No measurable change; one settings read per import request

**Constraints**: No new dependency, no schema change, no change to import validation or HTTP responses

**Scale/Scope**: One settings card, three routes, three view components, two catalogs

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **User Data Sovereignty**: PASS — import behavior, selection, and data are unchanged.
- **Safe by Default**: PASS — no secrets are read or logged; TLS rules are unchanged.
- **Provider Neutrality**: PASS — no provider-specific logic is added.
- **Test-Gated, Reversible Changes**: PASS — catalog parity, handler, and view tests cover the change; it is reversible by reverting the commit.
- **Boring, Reviewable Craft**: PASS — reuses the existing i18n helpers and templ components; no new dependency.

## Project Structure

### Documentation (this feature)

```text
specs/001-export-import-i18n/
├── spec.md
├── plan.md
└── tasks.md
```

### Source Code (repository root)

```text
internal/i18n/messages_en.go          # English source catalog
internal/i18n/messages_fr.go          # French catalog
internal/handler/thunderbird_import.go
internal/handler/user_settings.go     # uiLocale helper
internal/thunderbird/prefs.go         # warning codes
internal/views/thunderbird_import.templ
internal/views/export_import_tab.templ
internal/views/settings.templ
```

**Structure Decision**: Extend the existing single-module Go application. Locale resolution stays in the handler; presentation stays in templ views; translations stay in the i18n catalogs.

## Complexity Tracking

No constitution violations.
