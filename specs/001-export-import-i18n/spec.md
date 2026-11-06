# Feature Specification: Localize the Thunderbird import experience

**Feature Branch**: `001-export-import-i18n`

**Created**: 2026-11-06

**Status**: Implemented (commit `73d1de36`)

**Input**: User description: "Localize the Export and Import settings tab in English and French" (scoped to the Thunderbird import experience)

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Use the Thunderbird import in French (Priority: P1)

A French-speaking administrator opens Settings → Export and import, sees the Thunderbird card, opens the import dialog, uploads a `prefs.js` or profile ZIP, reviews the accounts, and imports them. Every label, help text, button, status, warning, and result message appears in French. The underlying import behavior is unchanged.

**Why this priority**: The app already offers a French interface. The import flow is the newest user-facing surface and currently falls back to English, which breaks the experience for the primary French audience.

**Independent Test**: Set the interface language to French, complete a Thunderbird import from upload through result, and verify that no English UI strings remain in the flow while the imported accounts match the English run.

**Acceptance Scenarios**:

1. **Given** the interface language is French, **When** the user opens the Thunderbird import card and dialog, **Then** the card title, description, button, file label, help text, and dialog title are French.
2. **Given** a valid Thunderbird file in French, **When** the preview renders, **Then** account statuses, status descriptions, warnings, signature count, and action buttons are French.
3. **Given** a successful import in French, **When** the result renders, **Then** added, failed, skipped, and signature counts are French and grammatically correct.
4. **Given** the interface language is English, **When** the same flow runs, **Then** every string remains in English and behaves as before.

---

### User Story 2 - Understand import errors in the current language (Priority: P2)

A user uploads an invalid, oversized, or unsupported file, or an import fails partway. The error or warning is shown in the user's interface language instead of a hardcoded English message.

**Why this priority**: Errors are where translation matters most; an English error in an otherwise French flow is confusing and hides the recovery action.

**Independent Test**: Trigger each Thunderbird error path (not a prefs file, too large, no accounts, unsupported plaintext server, generic import failure) in French and verify the message is French and still actionable.

**Acceptance Scenarios**:

1. **Given** a file that is not a Thunderbird prefs file, **When** the preview request fails, **Then** the French error explains what file to choose.
2. **Given** a file above the size limit, **When** it is uploaded, **Then** the French error states the limit.
3. **Given** a profile with no importable accounts, **When** it is previewed, **Then** the French message explains that no accounts were found.
4. **Given** a plaintext IMAP/SMTP account, **When** it is previewed, **Then** the French status explains why it is skipped.

---

### User Story 3 - Keep catalogs complete and safe to extend (Priority: P3)

A developer adds or changes a Thunderbird string. The English catalog is the source, the French catalog must stay in parity, and a missing key falls back to English rather than showing the key name.

**Why this priority**: Without a parity check, the next feature silently reintroduces English text into the French experience.

**Independent Test**: Remove a key from one catalog and run the i18n tests; the parity test fails. Request an unknown locale and verify English fallback.

**Acceptance Scenarios**:

1. **Given** the English and French catalogs, **When** tests run, **Then** every English key has a non-empty French translation and no unknown keys exist.
2. **Given** an unsupported locale, **When** the Thunderbird import renders, **Then** English is used and no raw key names appear.

---

### Edge Cases

- The user changes the interface language while an import dialog is open: the next server response uses the newly stored locale.
- The locale cannot be resolved before a request fails (for example, an unauthenticated or malformed request): the response falls back to English and remains readable.
- A count is zero or one: French and English plural forms are correct ("1 compte" vs "2 comptes", "1 signature" vs "2 signatures").
- A warning contains an account email, host, or provider name: those values are interpolated, not translated, and remain correctly escaped.
- A translation contains `%` or braces: formatting uses the existing `Tf` helper and does not panic on unexpected placeholders.
- The English flow must remain byte-for-byte behaviorally identical apart from the text source.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST resolve the current user's interface locale for every Thunderbird import route (upload form, preview, import) in open, personal, and managed authentication modes.
- **FR-002**: All static strings in the Thunderbird card, upload form, preview, result, and dialog MUST come from the i18n catalogs, not hardcoded English.
- **FR-003**: Server-generated Thunderbird messages (validation errors, unsupported-account reasons, warnings, and result text) MUST be localized before rendering.
- **FR-004**: The English catalog MUST remain the source of truth; every new key MUST have a French translation.
- **FR-005**: The i18n parity test MUST fail when a key is missing, empty, or unknown in any supported catalog.
- **FR-006**: An unknown or unavailable locale MUST fall back to English without exposing catalog keys.
- **FR-007**: Counts MUST use locale-aware singular/plural forms; the implementation MUST NOT build sentences by concatenating translated fragments.
- **FR-008**: User-provided values (email, display name, host, provider) MUST be interpolated and escaped without translation.
- **FR-009**: Import behavior, validation rules, account selection, signature handling, and HTTP contracts MUST NOT change.
- **FR-010**: The feature MUST NOT add a runtime dependency or require a new background service.

### Key Entities *(include if feature involves data)*

- **Locale**: The interface language stored in user UI settings; currently `en` and `fr`, with English fallback.
- **Catalog key**: A stable identifier in `internal/i18n` whose English value is the source and whose French value is the translation.
- **Thunderbird preview**: The account list, warnings, and signature count shown before import.
- **Thunderbird result**: Added, failed, skipped, and signature counts shown after import.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of user-visible strings in the Thunderbird import flow resolve through the i18n catalogs; a grep for hardcoded English in the flow returns no user-facing literals.
- **SC-002**: A French end-to-end smoke test (upload → preview → import → result) shows no English UI text.
- **SC-003**: The existing English flow passes all current handler and view tests without behavior changes.
- **SC-004**: The i18n parity test covers every new key and fails on an intentionally missing translation.
- **SC-005**: Error paths for invalid, oversized, empty, and unsupported files show localized, actionable messages in French.

## Assumptions

- The existing `internal/i18n` package and user UI settings are the source of the locale; no new locale storage is introduced.
- Only English and French are in scope; adding a third language is a catalog exercise, not a redesign.
- The Export and Import tab's profile and mail sections remain English in this iteration; the Thunderbird card and its dialogs are the bounded scope.
- Provider names (Gmail, Outlook, IMAP, SMTP) and protocol acronyms stay untranslated.
- The existing HTTP endpoints, form fields, and response shapes remain stable.
