# Tasks: Localize the Import mail experience

**Input**: `/specs/004-mail-import-i18n/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md)

## Phase 1: Setup

- [x] T001 Inventory Import mail strings
- [x] T002 Add English keys in `internal/i18n/messages_en.go`

## Phase 2: Foundational

- [x] T003 Add French translations in `internal/i18n/messages_fr.go`
- [x] T004 Localize `mailImportAccountProblem` and its callers
- [x] T005 Resolve the owner locale in import handlers

## Phase 3: User Story 1 — Import mail in French (P1) 🎯 MVP

- [x] T006 Localize `MailImportUploadForm`
- [x] T007 Localize `MailImportReview`, source rows, account heading, and destination selectors
- [x] T008 Localize `MailImportJobs`, `mailImportJobRow`, and `mailImportJobStatus`
- [x] T009 Localize `mailFolderTargetSelect` placeholders

## Phase 4: User Story 2 — Understand import errors (P2)

- [x] T010 Localize upload and start errors in `mail_import_routes.go`
- [x] T011 Add `TestMailImportRendersFrench`

## Phase 5: User Story 3 — Keep catalogs complete (P3)

- [x] T012 Verify catalog parity covers the new keys
- [x] T013 Run `templ generate`, `go test ./...`, and `go vet ./...`

## Dependencies & Execution Order

Phase 1 → 2 → 3 → 4 → 5. T007 depends on T003/T004; T010 depends on T003/T005.

## Implementation Strategy

Localize the upload and review first, then job statuses, then errors. Copy or move mail remains English until its own feature.
