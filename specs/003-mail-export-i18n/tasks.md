# Tasks: Localize the Mail export experience

**Input**: `/specs/003-mail-export-i18n/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md)

## Phase 1: Setup

- [x] T001 Inventory Mail card and Export mail strings
- [x] T002 Add English keys in `internal/i18n/messages_en.go`

## Phase 2: Foundational

- [x] T003 Add French translations in `internal/i18n/messages_fr.go`
- [x] T004 Add `mailLocaleCount` and locale-aware job status helpers
- [x] T005 Resolve the owner locale in export handlers

## Phase 3: User Story 1 — Export mail in French (P1) 🎯 MVP

- [x] T006 Localize the Mail card and Export mail dialog title in `export_import_tab.templ`
- [x] T007 Localize `MailExportPanel` and `mailExportAccountSummary`
- [x] T008 Localize `MailExportForm`, folder choices, saved-only option, and actions
- [x] T009 Localize `MailExportJobs`, `mailExportJobRow`, and `mailExportJobStatus`

## Phase 4: User Story 2 — Understand export errors (P2)

- [x] T010 Localize account-gone, folder-list, no-folder, and start-failure messages
- [x] T011 Add French view coverage for the panel, form, and completed job

## Phase 5: User Story 3 — Keep catalogs complete (P3)

- [x] T012 Verify catalog parity covers the new keys
- [x] T013 Run `templ generate`, `go test ./...`, and `go vet ./...`

## Dependencies & Execution Order

Phase 1 → 2 → 3 → 4 → 5. T007 depends on T003/T004; T010 depends on T003/T005.

## Implementation Strategy

Deliver the card and export flow first, then error states, then catalog guardrails. Import and copy/move remain English until their own feature.
