# Feature Specification: Localize the Mail export experience

**Feature Branch**: `003-mail-export-i18n`

**Created**: 2026-11-06

**Status**: Implemented

**Input**: User description: "Localize the Mail card and the Export mail flow in English and French"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Export mail in French (Priority: P1)

A French-speaking administrator opens Settings → Export and import → Mail, chooses an account, selects folders, starts an export, and downloads the archive. The card, account list, folder choices, progress, result, and actions are French. The archive contents and job behavior are unchanged.

**Why this priority**: The Mail card is visible on the tab, and export is the first mail action a user needs.

**Independent Test**: Set the interface language to French, run an export from folder selection to download, and verify every visible label and status is French while the ZIP and mbox contents match an English run.

**Acceptance Scenarios**:

1. **Given** French is selected, **When** the Mail card and Export mail dialog open, **Then** the title, description, account list, folder labels, options, and buttons are French.
2. **Given** a running export in French, **When** the job list refreshes, **Then** waiting, progress, pause, cancellation, completion, and failure summaries are French.
3. **Given** a completed export in French, **When** the job row renders, **Then** Download, Delete, and the archive summary are French.
4. **Given** English is selected, **When** the same flow runs, **Then** the text and behavior remain as before.

---

### User Story 2 - Understand export errors in the current language (Priority: P2)

A user opens an export for an account that was removed, cannot list folders, starts without choosing a folder, or starts an export that fails. The message is in the current language.

**Why this priority**: Export failures otherwise leave the user without a next step.

**Independent Test**: Trigger the account-gone, folder-list, no-folder, and start-failure paths in French and verify each message is French.

**Acceptance Scenarios**:

1. **Given** an account that no longer exists, **When** the export dialog opens, **Then** the French message says it is no longer in Gofer.
2. **Given** no folder is selected, **When** the export starts, **Then** the French message asks for at least one folder.
3. **Given** the folder list fails, **When** the form renders, **Then** the French message says the folders could not be listed.

---

### User Story 3 - Keep catalogs complete (Priority: P3)

The English and French catalogs stay in parity, and an unknown locale falls back to English.

**Independent Test**: Remove a mail export key from the French catalog; the parity test fails. Render with an unknown locale; English is used.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The Mail card title, description, Export mail row, and Export mail dialog title MUST use the i18n catalogs.
- **FR-002**: The export panel, form, folder list, options, job list, and job statuses MUST be localized.
- **FR-003**: Export errors for account-gone, folder-list, no-folder, and start-failure MUST be localized.
- **FR-004**: Counts MUST use locale-aware singular/plural forms.
- **FR-005**: The English catalog MUST remain the source; every new key MUST have a French translation.
- **FR-006**: The i18n parity test MUST cover the new keys.
- **FR-007**: Export behavior, ZIP/mbox format, job state machine, and HTTP contracts MUST NOT change.
- **FR-008**: The feature MUST NOT add a runtime dependency or a schema migration.

### Key Entities

- **Mail export job**: account, folders, progress, exported/failed/not-saved counts, state, start time.
- **Export form**: account, selected folders, saved-only option, error message.
- **Locale**: `en` or `fr`, English fallback.

## Success Criteria *(mandatory)*

- **SC-001**: 100% of user-visible strings in the Mail card and Export mail flow resolve through the i18n catalogs.
- **SC-002**: A French export smoke test shows no English UI text.
- **SC-003**: The English export tests pass unchanged.
- **SC-004**: The parity test covers every new key.
- **SC-005**: Account-gone, folder-list, no-folder, and start-failure messages are French.

## Assumptions

- The existing `internal/i18n` package and `uiLocale` helper are reused.
- Only English and French are in scope.
- Import mail and Copy or move mail remain English in this iteration.
- Engine diagnostics shown in job errors may stay in English; the surrounding UI is localized.
- Provider names, folder paths, and archive formats stay untranslated.
