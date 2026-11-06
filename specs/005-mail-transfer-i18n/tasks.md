# Tasks: Localize the Copy or move mail experience

**Input**: `/specs/005-mail-transfer-i18n/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md)

## Phase 1: Setup

- [x] T001 Inventory Copy or move strings
- [x] T002 Add English keys in `internal/i18n/messages_en.go`

## Phase 2: Foundational

- [x] T003 Add French translations in `internal/i18n/messages_fr.go`
- [x] T004 Localize `mailTransferSourceProblem` and its callers
- [x] T005 Resolve the owner locale in transfer handlers

## Phase 3: User Story 1 — Transfer mail in French (P1) 🎯 MVP

- [x] T006 Localize `MailTransferPanel` and `mailTransferAccountSelect`
- [x] T007 Localize `MailTransferForm` modes, note, and start action
- [x] T008 Localize `mailTransferFolderRow` and destination selectors
- [x] T009 Localize `MailTransferJobs`, `mailTransferJobRow`, and `mailTransferJobStatus`

## Phase 4: User Story 2 — Understand transfer errors (P2)

- [x] T010 Localize source/target, same-account, no-folder, and folder-gone errors
- [x] T011 Add `TestMailTransferRendersFrench`

## Phase 5: User Story 3 — Keep catalogs complete (P3)

- [x] T012 Verify catalog parity covers the new keys
- [x] T013 Run `templ generate`, `go test ./...`, and `go vet ./...`

## Dependencies & Execution Order

Phase 1 → 2 → 3 → 4 → 5. T006–T009 depend on T003; T010 depends on T004/T005.

## Implementation Strategy

Localize the form first, then job statuses, then errors. The message-list "Move to another account" dialog remains for its own feature.
