# Tasks: Localize the Profile transfer experience

**Input**: Design documents from `/specs/002-profile-transfer-i18n/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md)

**Tests**: Included because the specification requires catalog parity and French rendering coverage.

## Phase 1: Setup

- [x] T001 Inventory user-visible Profile strings in the card, export form, upload, preview, conflicts, result, and errors
- [x] T002 Add English source keys in `internal/i18n/messages_en.go`

---

## Phase 2: Foundational

- [x] T003 Add French translations in `internal/i18n/messages_fr.go`
- [x] T004 Add `errProfileNotProfile`, `errProfileDamaged`, and `errProfileNewer` sentinels in `internal/handler/profile_transfer.go`
- [x] T005 Add locale-aware date, list, and count helpers in handlers and views

**Checkpoint**: Catalogs, parser codes, and formatting helpers ready.

---

## Phase 3: User Story 1 — Use Profile transfer in French (P1) 🎯 MVP

- [x] T006 Localize the Profile card and dialog titles in `internal/views/export_import_tab.templ`
- [x] T007 Localize `ProfileExportForm` and pass `uiSettings`
- [x] T008 Localize `ProfileImportUploadForm` and the upload route
- [x] T009 Localize preview sections, account statuses, contacts, and conflict controls
- [x] T010 Localize `ProfileImportResult`
- [x] T011 Format the exported date in French

**Checkpoint**: French export and import flows work end to end.

---

## Phase 4: User Story 2 — Understand profile errors (P2)

- [x] T012 Map parser sentinels and upload errors to localized messages in `internal/handler/profile_transfer_routes.go`
- [x] T013 Localize import-stopped and storage-unavailable messages
- [x] T014 Add `TestProfileErrorsFollowLocale` and `TestProfilePreviewRouteUsesFrench`

**Checkpoint**: Error paths are localized.

---

## Phase 5: User Story 3 — Keep catalogs complete (P3)

- [x] T015 Verify catalog parity covers the new keys
- [x] T016 Add `TestProfileTransferRendersFrench` for the card, export form, upload form, and result
- [x] T017 Run `templ generate`, `go test ./...`, and `go vet ./...`

---

## Dependencies & Execution Order

- Phase 1 → Phase 2 → Phase 3 → Phase 4 → Phase 5.
- T008 and T009 depend on T003 and T005; T012 depends on T004 and T003.
- Tests T014 and T016 depend on the localized view and handler signatures.

## Implementation Strategy

Deliver the P1 flow first, then errors, then catalog guardrails. Verify the English behavior with the existing profile tests and the full suite.
