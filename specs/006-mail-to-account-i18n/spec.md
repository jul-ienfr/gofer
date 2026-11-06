# Feature Specification: Localize the Move to another account dialog

**Feature Branch**: `006-mail-to-account-i18n`

**Created**: 2026-11-06

**Status**: Implemented

**Input**: User description: "Localize the Move to another account dialog in English and French"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Move mail from the message list in French (Priority: P1)

A French-speaking user selects messages, chooses a folder in another account, and confirms the move. The dialog title, summary, move/copy choices, confirm button, background note, and errors are French. The messages move exactly as before.

**Independent Test**: Set the interface language to French, choose Move to another account from the message list, and verify every visible label and summary is French while the resulting transfer matches an English run.

**Acceptance Scenarios**:

1. **Given** French is selected, **When** the dialog opens, **Then** its title, summary, mode labels and descriptions, background note, and Cancel are French.
2. **Given** a different mode is chosen, **When** the confirm button updates, **Then** its label is French.
3. **Given** a folder that cannot receive mail or an account waiting for a password, **When** the dialog opens, **Then** the problem is French.
4. **Given** English is selected, **When** the same flow runs, **Then** the text and behavior remain as before.

---

### User Story 2 - Understand confirmation errors in the current language (Priority: P2)

A user submits no messages, a folder that cannot take mail, an account waiting for a password, or messages already in the target account. The error is in the current language.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The dialog title, summary, mode labels and descriptions, background note, Cancel, and confirm button MUST use the i18n catalogs.
- **FR-002**: Dialog problems and confirmation errors MUST be localized.
- **FR-003**: Counts MUST use locale-aware singular/plural forms.
- **FR-004**: The English catalog MUST remain the source; every new key MUST have a French translation.
- **FR-005**: The parity test MUST cover the new keys.
- **FR-006**: Move/copy behavior and HTTP contracts MUST NOT change.
- **FR-007**: The feature MUST NOT add a runtime dependency or schema migration.

### Key Entities

- **Dialog data**: count, sources, target, folder, problem, default mode.
- **Locale**: `en` or `fr`, English fallback.

## Success Criteria *(mandatory)*

- **SC-001**: 100% of user-visible strings in the dialog resolve through the i18n catalogs.
- **SC-002**: A French dialog smoke test shows no English UI text.
- **SC-003**: Existing English tests pass unchanged.
- **SC-004**: Parity covers every new key.
- **SC-005**: Problem and confirmation errors are French.

## Assumptions

- The existing `internal/i18n` package and `uiLocale` helper are reused.
- Only English and French are in scope.
- Account addresses and folder paths stay untranslated.
