# Tasks: Localize the Thunderbird import experience

**Input**: Design documents from `/specs/001-export-import-i18n/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md)

**Tests**: Included because the specification requires catalog parity and French rendering coverage.

**Organization**: Tasks are grouped by user story. All tasks are complete as of commit `73d1de36`.

## Phase 1: Setup

**Purpose**: Confirm the existing i18n framework and identify the strings in scope.

- [x] T001 Review `internal/i18n` helpers (`T`, `Tf`, `TP`) and the settings locale map
- [x] T002 Inventory user-visible strings in the Thunderbird card, dialog, preview, result, and errors
- [x] T003 Add English source keys in `internal/i18n/messages_en.go`

---

## Phase 2: Foundational

**Purpose**: Locale resolution and warning metadata shared by all stories.

- [x] T004 Add `uiLocale(ctx, owner)` in `internal/handler/user_settings.go` with English fallback
- [x] T005 Add warning codes to `internal/thunderbird/prefs.go` while keeping parser messages
- [x] T006 Add French translations in `internal/i18n/messages_fr.go`
- [x] T007 Add count, list-conjunction, and failure-message keys shared by preview and result

**Checkpoint**: Catalogs and locale resolution ready.

---

## Phase 3: User Story 1 — Use the Thunderbird import in French (P1) 🎯 MVP

**Goal**: The full import flow renders in the owner's language.

- [x] T008 Pass the locale to `ThunderbirdImportUploadForm`, `ThunderbirdImportPreview`, and `ThunderbirdImportResult`
- [x] T009 Localize the card and dialog title in `internal/views/export_import_tab.templ`
- [x] T010 Localize status labels, descriptions, signatures, and action buttons in `internal/views/thunderbird_import.templ`
- [x] T011 Pass `uiSettings` from `SettingsPage` to `SettingsExportImportTab`

**Checkpoint**: French flow works end to end.

---

## Phase 4: User Story 2 — Understand import errors (P2)

**Goal**: Validation and import failures use the current language.

- [x] T012 Localize upload, ZIP, parser, and generic import errors in `internal/handler/thunderbird_import.go`
- [x] T013 Localize handler warnings (missing server, unsupported type, duplicates, OAuth fallback, missing SMTP, plaintext)
- [x] T014 Add a French preview route test covering status, counts, and actions in `internal/handler/thunderbird_import_test.go`

**Checkpoint**: Error and warning paths are localized.

---

## Phase 5: User Story 3 — Keep catalogs complete (P3)

**Goal**: Future changes cannot silently lose a translation.

- [x] T015 Verify the existing catalog parity test covers every new key
- [x] T016 Add French upload and result rendering tests in `internal/views/i18n_test.go`
- [x] T017 Run `templ generate`, `go test ./...`, and `go vet ./...`

---

## Dependencies & Execution Order

- Phase 1 → Phase 2 → Phase 3 → Phase 4 → Phase 5.
- T008 depends on T004 and T006; T012 depends on T005 and T006.
- Tests T014 and T016 depend on the corresponding view/handler signatures.

## Implementation Strategy

Deliver the P1 flow first (card, upload, preview, result), then error paths, then catalog guardrails. The implementation is complete and verified by the full test suite.
