# Implementation Plan: Localize the Profile transfer experience

**Branch**: `002-profile-transfer-i18n` | **Date**: 2026-11-06 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/002-profile-transfer-i18n/spec.md`

## Summary

Translate the Profile card, export dialog, import upload, preview, conflict controls, result, and error paths through the existing `internal/i18n` catalogs. Keep the profile JSON format, import rules, and HTTP endpoints unchanged. Resolve the owner's locale once per request and pass it to the views; use stable parser error sentinels for translated messages.

## Technical Context

**Language/Version**: Go 1.27

**Primary Dependencies**: templ, templUI, Tailwind CSS, HTMX, existing `internal/i18n` package

**Storage**: SQLite UI settings (`language`) through the existing per-user store

**Testing**: `go test ./...`, `go vet ./...`, templ generation

**Target Platform**: Self-hosted web application (single binary)

**Project Type**: Web service with server-rendered views

**Performance Goals**: No measurable change; one settings read per profile request

**Constraints**: No new dependency, no schema change, no change to the profile JSON schema or import behavior

**Scale/Scope**: One settings card, two dialogs, three routes, two catalogs

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **User Data Sovereignty**: PASS — export/import data and choices are unchanged.
- **Safe by Default**: PASS — no credentials are exported or imported; validation is unchanged.
- **Provider Neutrality**: PASS — provider-specific behavior is untouched.
- **Test-Gated, Reversible Changes**: PASS — handler, view, catalog-parity, and French tests cover the change.
- **Boring, Reviewable Craft**: PASS — reuses existing helpers and components; no new dependency.

## Project Structure

### Documentation (this feature)

```text
specs/002-profile-transfer-i18n/
├── spec.md
├── plan.md
└── tasks.md
```

### Source Code (repository root)

```text
internal/i18n/messages_en.go
internal/i18n/messages_fr.go
internal/handler/profile_transfer.go        # stable parser error sentinels
internal/handler/profile_transfer_routes.go # locale, errors, date formatting
internal/views/export_import_tab.templ      # Profile card and export form
internal/views/profile_transfer.templ       # upload, preview, conflicts, result
internal/handler/profile_transfer_test.go
internal/views/i18n_test.go
```

**Structure Decision**: Extend the existing single-module Go application. Locale resolution stays in handlers; presentation stays in templ views; translations stay in the catalogs.

## Complexity Tracking

No constitution violations.
