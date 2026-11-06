# Feature Specification: Localize the Copy or move mail experience

**Feature Branch**: `005-mail-transfer-i18n`

**Created**: 2026-11-06

**Status**: Implemented

**Input**: User description: "Localize the Copy or move mail flow in English and French"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Transfer mail in French (Priority: P1)

A French-speaking administrator opens Settings → Export and import → Mail → Copy or move mail, chooses a source and destination account, selects copy or move, picks folders and destinations, starts the transfer, and follows the job. All of it is French. Transfer behavior and message counts are unchanged.

**Why this priority**: Copy or move is the last Mail action still in English.

**Independent Test**: Set the interface language to French, start a copy between two accounts, and verify every visible label, mode description, folder selector, status, and action is French while the transferred messages match an English run.

**Acceptance Scenarios**:

1. **Given** French is selected, **When** the Copy or move dialog opens, **Then** the account labels, mode names and descriptions, folder heading, note, and Start button are French.
2. **Given** a running transfer in French, **When** the job list refreshes, **Then** waiting, progress, pause, cancellation, copied, duplicate, failure, and deletion summaries are French.
3. **Given** a transfer started from the message list, **When** it runs in the background, **Then** the settings job list is French.
4. **Given** English is selected, **When** the same flow runs, **Then** the text and behavior remain as before.

---

### User Story 2 - Understand transfer errors in the current language (Priority: P2)

A user chooses a disconnected account, the same account twice, no folder, a missing new-folder name, or a folder that disappeared. The message is in the current language.

**Independent Test**: Trigger each start error in French and verify the message is French.

**Acceptance Scenarios**:

1. **Given** a source waiting for a password, **When** the transfer starts, **Then** the French message asks for its password.
2. **Given** the same account twice, **When** the transfer starts, **Then** the French message asks for two different accounts.
3. **Given** no folder, **When** the transfer starts, **Then** the French message asks for at least one folder.
4. **Given** a missing new-folder name, **When** the transfer starts, **Then** the French message asks to name each new folder.

---

### User Story 3 - Keep catalogs complete (Priority: P3)

English and French catalogs stay in parity; an unknown locale falls back to English.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The Copy or move panel, account selects, mode choices, folder rows, and job list MUST use the i18n catalogs.
- **FR-002**: Transfer account problems (waiting for a password) MUST be localized.
- **FR-003**: Transfer start errors MUST be localized.
- **FR-004**: Counts MUST use locale-aware singular/plural forms.
- **FR-005**: The English catalog MUST remain the source; every new key MUST have a French translation.
- **FR-006**: The parity test MUST cover the new keys.
- **FR-007**: Transfer behavior, copy/move/delete semantics, and HTTP contracts MUST NOT change.
- **FR-008**: The feature MUST NOT add a runtime dependency or schema migration.

### Key Entities

- **Transfer job**: source, target, mode, folders, counts, state, start time.
- **Transfer form**: source, target, mode, folder destinations.
- **Locale**: `en` or `fr`, English fallback.

## Success Criteria *(mandatory)*

- **SC-001**: 100% of user-visible strings in the Copy or move flow resolve through the i18n catalogs.
- **SC-002**: A French transfer smoke test shows no English UI text.
- **SC-003**: Existing English transfer tests pass unchanged.
- **SC-004**: Parity covers every new key.
- **SC-005**: Start error paths are French.

## Assumptions

- The existing `internal/i18n` package and `uiLocale` helper are reused.
- Only English and French are in scope.
- The message-list "Move to another account" dialog has its own feature.
- Engine diagnostics may stay in English; the surrounding UI is localized.
- Account addresses and folder paths stay untranslated.
