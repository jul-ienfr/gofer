# Feature Specification: Localize the Import mail experience

**Feature Branch**: `004-mail-import-i18n`

**Created**: 2026-11-06

**Status**: Implemented

**Input**: User description: "Localize the Import mail flow in English and French"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Import mail in French (Priority: P1)

A French-speaking administrator opens Settings → Export and import → Mail → Import mail, uploads mbox/eml/ZIP files, chooses an account and where each set of messages goes, starts the import, and follows the job. The upload form, review, account choices, folder selectors, progress, result, and actions are French. Imported messages and job behavior are unchanged.

**Why this priority**: Import is the second mail action and the upload/review screens are still English.

**Independent Test**: Set the interface language to French, import a small mbox into an account, and verify every visible label, selector, status, and result is French while the imported messages match an English run.

**Acceptance Scenarios**:

1. **Given** French is selected, **When** the Import mail dialog opens, **Then** Files, Upload and review, and the multi-file hint are French.
2. **Given** uploaded files in French, **When** the review renders, **Then** the account heading, source summary, destination heading, folder placeholders, and start/cancel actions are French.
3. **Given** a running import in French, **When** the job list refreshes, **Then** waiting, progress, pause, cancellation, import, duplicate, failure, and destination summaries are French.

---

### User Story 2 - Understand import errors in the current language (Priority: P2)

A user uploads no file, an unreadable file, an empty archive, a bad ZIP, an expired upload, no account, or a target that disappeared. The message is in the current language.

**Independent Test**: Trigger each upload and start error path in French and verify the message is French and actionable.

**Acceptance Scenarios**:

1. **Given** no file, **When** the upload is submitted, **Then** the French message asks for files.
2. **Given** a ZIP that cannot be read, **When** it is uploaded, **Then** the French message says the ZIP could not be read.
3. **Given** an expired upload, **When** the review or start runs, **Then** the French message asks to upload again.
4. **Given** a missing new-folder name, **When** the import starts, **Then** the French message asks to name each new folder.

---

### User Story 3 - Keep catalogs complete (Priority: P3)

English and French catalogs stay in parity; an unknown locale falls back to English.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The Import mail upload form, review, destination selectors, and job list MUST use the i18n catalogs.
- **FR-002**: Account problems (waiting for a password) MUST be localized.
- **FR-003**: Upload and start errors MUST be localized.
- **FR-004**: Counts MUST use locale-aware singular/plural forms.
- **FR-005**: The English catalog MUST remain the source; every new key MUST have a French translation.
- **FR-006**: The parity test MUST cover the new keys.
- **FR-007**: Import behavior, mbox/eml/ZIP parsing, deduplication, and HTTP contracts MUST NOT change.
- **FR-008**: The feature MUST NOT add a runtime dependency or schema migration.

### Key Entities

- **Import job**: files, sources, account, folders, counts, state, start time.
- **Review**: account, destination per source, new-folder names.
- **Locale**: `en` or `fr`, English fallback.

## Success Criteria *(mandatory)*

- **SC-001**: 100% of user-visible strings in the Import mail flow resolve through the i18n catalogs.
- **SC-002**: A French import smoke test shows no English UI text.
- **SC-003**: Existing English import tests pass unchanged.
- **SC-004**: Parity covers every new key.
- **SC-005**: Upload and start error paths are French.

## Assumptions

- The existing `internal/i18n` package and `uiLocale` helper are reused.
- Only English and French are in scope.
- Copy or move mail remains English until its own feature.
- Engine diagnostics may stay in English; the surrounding UI is localized.
- File formats and folder paths stay untranslated.
