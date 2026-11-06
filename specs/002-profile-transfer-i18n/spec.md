# Feature Specification: Localize the Profile transfer experience

**Feature Branch**: `002-profile-transfer-i18n`

**Created**: 2026-11-06

**Status**: Implemented

**Input**: User description: "Localize the Profile section of the Export and Import settings tab in English and French"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Export and import a profile in French (Priority: P1)

A French-speaking administrator opens Settings → Export and import → Profile, exports a profile, then imports one. The card, export choices, upload form, preview sections, account statuses, contact conflicts, and result all appear in French. The underlying export and import behavior is unchanged.

**Why this priority**: The card and both dialogs are immediately visible on the Export and import tab, and they are still entirely English in the French interface.

**Independent Test**: Set the interface language to French, export a profile, import it back, and verify that every visible label, choice, status, and result is French while the exported file and imported data are identical to an English run.

**Acceptance Scenarios**:

1. **Given** French is selected, **When** the Profile card and export dialog open, **Then** the title, description, checkbox, help text, and buttons are French.
2. **Given** a valid Gofer profile in French, **When** the preview renders, **Then** the exported date, section titles, counts, account statuses, and contact conflict choices are French.
3. **Given** a successful import in French, **When** the result renders, **Then** preferences, signatures, contacts, accounts, replacements, and failures are described in French.
4. **Given** English is selected, **When** the same flow runs, **Then** the text and behavior remain as before.

---

### User Story 2 - Understand profile errors in the current language (Priority: P2)

A user chooses the wrong file, an oversized file, a damaged or newer profile, or an import fails partway. The message is shown in the current interface language and still explains how to recover.

**Why this priority**: Profile import replaces settings and contacts; a misunderstood error can lead to repeated imports or lost confidence.

**Independent Test**: Trigger the invalid, oversized, damaged, newer-version, and storage-failure paths in French and verify each message is French and actionable.

**Acceptance Scenarios**:

1. **Given** a file that is not a Gofer profile, **When** it is uploaded, **Then** the French error says the file is not a Gofer profile.
2. **Given** a damaged profile, **When** it is uploaded, **Then** the French error says the file is damaged.
3. **Given** a profile from a newer Gofer version, **When** it is uploaded, **Then** the French error asks the user to update Gofer.
4. **Given** storage is unavailable during import, **When** the import stops, **Then** the French error asks the user to try again later.

---

### User Story 3 - Keep catalogs complete (Priority: P3)

A developer adding a profile string must keep English and French in parity, and an unknown locale must fall back to English rather than showing a key.

**Why this priority**: The same guardrails added for the Thunderbird flow must cover the Profile flow.

**Independent Test**: Remove a profile key from the French catalog and run the i18n tests; the parity test fails. Render the card with an unknown locale and verify English.

**Acceptance Scenarios**:

1. **Given** both catalogs, **When** tests run, **Then** every profile key has a non-empty translation and no unknown keys exist.
2. **Given** an unsupported locale, **When** the Profile card renders, **Then** English is used and no raw key appears.

### Edge Cases

- A profile contains no preferences, signatures, contacts, or accounts: the preview omits those sections without leaving empty labels.
- A count is zero or one: French and English singular/plural forms are correct.
- A contact conflict is matched by email or by name: the "In Gofer" label reflects the reason in the current language.
- The user changes the language while a dialog is open: the next server response uses the new locale.
- Account emails, contact names, dates, and provider names are interpolated and escaped without translation.
- The exported JSON file is byte-compatible with the English flow.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST resolve the owner's locale for the Profile card, export form, upload form, preview, and import result.
- **FR-002**: All static strings in the Profile card, export dialog, import dialog, preview, conflict controls, and result MUST come from the i18n catalogs.
- **FR-003**: Server-generated profile errors and import failure messages MUST be localized.
- **FR-004**: Profile parser errors MUST use stable codes so they can be translated without matching English text.
- **FR-005**: The English catalog MUST remain the source; every new key MUST have a French translation.
- **FR-006**: The i18n parity test MUST cover the new keys.
- **FR-007**: Counts MUST use locale-aware singular/plural forms; list conjunctions MUST be localized.
- **FR-008**: User-provided values MUST be interpolated and escaped without translation.
- **FR-009**: Export/import behavior, JSON format, validation rules, contact conflict defaults, and HTTP contracts MUST NOT change.
- **FR-010**: The feature MUST NOT add a runtime dependency or a schema migration.

### Key Entities

- **Locale**: The interface language stored in user UI settings; `en` and `fr`, English fallback.
- **Profile preview**: Preferences, signatures, contacts, conflicts, and accounts shown before import.
- **Profile result**: Preferences, signatures, contacts, replacements, accounts, and failures after import.
- **Conflict**: A contact that may already exist, matched by email or name.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of user-visible strings in the Profile transfer flow resolve through the i18n catalogs.
- **SC-002**: A French end-to-end smoke test (export → upload → preview → import → result) shows no English UI text.
- **SC-003**: The English flow passes all existing profile tests without behavior changes.
- **SC-004**: The i18n parity test covers every new key and fails on a missing translation.
- **SC-005**: Invalid, oversized, damaged, newer-version, and storage-failure paths show localized messages in French.

## Assumptions

- The existing `internal/i18n` package and `uiLocale` handler helper are reused.
- Only English and French are in scope.
- The Mail section of the Export and import tab remains English in this iteration.
- Provider names and file formats (`prefs.js`, JSON, ZIP) stay untranslated.
- Existing HTTP endpoints and form field names remain stable.
