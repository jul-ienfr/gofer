# Tasks: Localize the Move to another account dialog

**Input**: `/specs/006-mail-to-account-i18n/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md)

## Phase 1: Setup

- [x] T001 Inventory dialog strings
- [x] T002 Add English keys in `internal/i18n/messages_en.go`

## Phase 2: Foundational

- [x] T003 Add French translations in `internal/i18n/messages_fr.go`
- [x] T004 Localize the dialog title, summary, modes, note, and actions
- [x] T005 Localize the confirm button through its data attribute
- [x] T006 Resolve the owner locale in the dialog and confirmation handlers

## Phase 3: Errors

- [x] T007 Localize folder, account, no-message, and already-there errors
- [x] T008 Add `TestMailToAccountDialogRendersFrench`

## Phase 4: Guardrails

- [x] T009 Verify catalog parity covers the new keys
- [x] T010 Run `templ generate`, `go test ./...`, and `go vet ./...`

## Dependencies & Execution Order

Phase 1 → 2 → 3 → 4. T004/T005 depend on T003; T007 depends on T006.

## Implementation Strategy

Translate the dialog first, then its errors, then add French coverage.
