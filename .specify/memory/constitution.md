<!--
Sync Impact Report
Version change: template -> 1.0.0
Modified principles: n/a (initial ratification)
Added sections: Core Principles, Technical Constraints, Development Workflow, Governance
Removed sections: none
Follow-up TODOs: none
-->

# Gofer Constitution

## Core Principles

### I. User Data Sovereignty

Gofer runs on the user's machine or server, and their data must remain theirs. Every feature that stores mail, contacts, calendars, accounts, or settings must preserve a supported export or backup path. Import features must be additive and must never silently overwrite existing data. Lock-in, hidden storage, and telemetry that exports user content are out of scope.

Rationale: Gofer is self-hosted webmail. Trust depends on users being able to leave with their data intact.

### II. Safe by Default

Mail connections require TLS or STARTTLS; plaintext transports are refused unless an explicit administrator exception exists. Credentials are encrypted at rest and are never written to logs, error messages, exports, or import previews. Destructive actions require explicit confirmation, and remote content stays blocked until the user allows it. Imports that cannot carry secrets must say so plainly.

Rationale: A self-hosted mail server holds the keys to every connected account. Safe defaults protect users from configuration mistakes and from tools that overreach.

### III. Provider Neutrality

Account behavior goes through the shared provider and account abstractions. Generic IMAP/SMTP is a first-class path, not a fallback; Gmail, Outlook, JMAP, DAV, and CalDAV integrate through the same account model. UI and handlers must not grow provider-specific branches when a shared capability can express the behavior. New providers must reuse existing sync, reconnect, credential, and export flows.

Rationale: Gofer supports many account types. Duplicated provider logic makes reconnect, migration, and security fixes inconsistent.

### IV. Test-Gated, Reversible Changes

Every behavior change includes tests at the closest useful level: parser and unit tests for pure logic, handler tests for HTTP and storage behavior, and view tests for rendered UI. The repository gates are `go test ./...` and `go vet ./...`; generated templ files must be regenerated with `templ generate` and kept in sync. Database migrations must have a rollback or restore path. A change that cannot be verified or reversed is not ready.

Rationale: Gofer is pre-1.0 and evolves quickly. Tests and reversible delivery keep that speed from becoming fragility.

### V. Boring, Reviewable Craft

Prefer the existing architecture, dependencies, and UI components over new ones. Keep diffs small and legible, name things after what they do, and document user-facing behavior in `docs/`. New dependencies and background services require a concrete justification. AI-assisted code is welcome but must be reviewed, tested, and understood by the person merging it.

Rationale: Mail software is long-lived infrastructure. Predictable code is easier to secure, debug, and hand to the next maintainer.

## Technical Constraints

- Go 1.27.3 or newer, templ views, Tailwind CSS, HTMX, and SQLite through `modernc.org/sqlite`.
- One self-contained binary with embedded assets for release builds; no required external runtime service.
- Generated `*_templ.go` files are committed and must match their `.templ` sources.
- Authentication modes are open, personal, and managed; features must work in each mode or explicitly state their scope.
- No telemetry or third-party analytics that receive user content.

## Development Workflow

1. Start bounded changes with a specification that names the user outcome and compatibility boundaries.
2. Plan against the existing packages and test conventions; call out migrations, security impact, and rollback.
3. Implement in small steps, regenerating templ after view changes and updating docs alongside behavior.
4. Run `go test ./...` and `go vet ./...`; run a manual smoke test for UI or import/export flows.
5. Review the diff for secrets, plaintext transports, and accidental changes to unrelated work before merging.

## Governance

This constitution governs feature specs, plans, tasks, and reviews. Amendments require a written proposal, a review of affected principles, and an update to this file through the Spec Kit constitution workflow. Versioning follows semantic versioning: MAJOR for removed or redefined principles, MINOR for new principles or material expansion, PATCH for clarifications. Compliance is checked in review; a deviation must be recorded with its justification and a follow-up task. Runtime development guidance lives in `README.md` and `docs/`.

**Version**: 1.0.0 | **Ratified**: 2026-11-04 | **Last Amended**: 2026-11-04
