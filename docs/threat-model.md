# Threat model and security assessment

This document is the Phase 8 threat model for the Gofer fork. It describes what the system protects, where trust changes, which abuse cases matter, and which controls are implemented. It is not a substitute for an independent penetration test.

## Scope

In scope:

- The Gofer HTTP application, including personal, managed, and open authentication modes.
- Mailbox, contacts, and Calendar synchronization.
- The local SQLite stores, attachments, encryption keys, and backup archives.
- AI analysis, draft generation, routing, and webhook delivery.
- Provider integrations (IMAP, SMTP, JMAP, Gmail, Outlook, CalDAV, CardDAV, OIDC).
- Deployment through a binary, Docker image, or Helm chart.

Out of scope:

- Compromise of the host operating system or hypervisor.
- Malicious mail providers or a compromised upstream provider account.
- Browser, operating-system, or password-manager vulnerabilities.
- Third-party AI providers retaining prompts or outputs under their own policies.
- Physical attacks against storage media.

## Assets

| Asset | Why it matters | Primary location |
| --- | --- | --- |
| Mailbox credentials and OAuth tokens | Full mailbox access; often reusable across services | Encrypted account rows in SQLite |
| Mail bodies, attachments, contacts, Calendar | Private communications and personal data | SQLite, attachment directory |
| Local login credentials, passkeys, TOTP seeds, recovery codes | Account takeover | Central authentication database |
| Session cookies and CSRF tokens | Authenticated access without a password | Browser and session store |
| `secret.key` or `GOFER_SECRET_KEY` | Decrypts every stored mailbox credential | Data directory or environment |
| AI prompts, drafts, analyses, connectors, API tokens | Private content and provider spend | SQLite and external AI provider |
| Webhook signing secrets | Forged automation events | Encrypted integration rows |
| Backup archives | Complete copy of the data above, including the key when it is in the data directory | Backup storage |
| Availability and integrity of synchronization | Lost mail, duplicate sends, stuck jobs | Workers, queues, SQLite |

## Actors and trust boundaries

| Actor | Trust | Capability |
| --- | --- | --- |
| Unauthenticated internet client | Untrusted | Reach public routes, login, setup, webhook endpoints |
| Authenticated user | Limited trust | Read and modify their own mail, contacts, Calendar, settings, drafts |
| Administrator (managed mode) | High trust | Manage users, policies, provider allowlists, security events; cannot read another user's private mail through admin screens |
| Local OS user | Host-level trust | Read the data directory, process memory, environment, and backups unless the OS denies it |
| Mail sender | Untrusted | Supply HTML, attachments, headers, calendar invitations, links |
| Mail/calendar provider | External, semi-trusted | Supply account data and OAuth identity claims |
| AI/LLM provider | External, semi-trusted | Receive prompts and return untrusted text or tool calls |
| Webhook receiver | External | Receive signed events |
| Webhook sender (Outlook) | Untrusted until signature validation | Post notifications to a public route |
| Supply chain | Untrusted | Go modules, npm assets, container base image, GitHub Actions |

Trust boundaries crossed by the application:

1. Browser to Gofer over HTTP or HTTPS.
2. Gofer to external providers over the network.
3. Authenticated user to another user's data.
4. User to administrator functions.
5. Gofer process to the data directory and environment.
6. Gofer to AI providers and webhook receivers.
7. Provider webhook to Gofer's public endpoint.
8. Build and release pipeline to shipped artifacts.

## Security objectives

- Confidentiality: only the owning user and explicitly authorized administrators can read private mail data.
- Integrity: messages, drafts, automation actions, and backups are not silently altered.
- Authentication: a Gofer session is not a mailbox credential, and a mailbox credential does not create a Gofer session.
- Authorization: every object access is scoped to the authenticated user or administrator role.
- Availability: a provider outage, webhook failure, or stuck AI job does not stop mail access or corrupt data.
- Recoverability: a verified backup can restore service within the RTO.
- Privacy: prompts and logs do not contain credentials, session tokens, or unnecessary message content.

## STRIDE analysis

| Category | Abuse case | Implemented controls | Residual risk |
| --- | --- | --- | --- |
| Spoofing | Attacker signs in with a stolen password or forged OAuth callback | Argon2id password hashing, common-password blocklist, login throttling, MFA with passkeys/TOTP, OAuth state and nonce validation, session rotation | A stolen valid session cookie or a compromised identity provider remains sufficient. Protect the host and use HTTPS. |
| Spoofing | Forged Outlook webhook triggers a sync | Webhook validation against the subscription and signed notification data | An attacker who can guess a valid subscription identifier can still cause unnecessary sync work; rate limits and provider validation reduce impact. |
| Tampering | User modifies another user's mail or settings | Per-user object ownership checks, 404 for cross-user access, per-user SQLite databases in managed mode, CSRF protection on state changes | A compromised administrator host can modify the database directly. |
| Tampering | Malicious archive overwrites files outside the data directory | Archive paths cleaned and joined under the target root; absolute paths, backslashes, `..`, symlinks, hardlinks, devices, duplicate entries, unlisted files, and unsafe database paths rejected | The operator can still choose any target directory. Run restore as the service user. |
| Tampering | Backup is silently corrupted | SHA-256 and size for every file in `manifest.json`; full verification before restore; duplicate manifest rejection | A malicious actor who can rewrite both the archive and the manifest can forge a self-consistent archive. Protect backup storage. |
| Repudiation | User denies changing a setting or sending mail | Security events and audit rows for authentication, administration, and AI actions; structured logs | Local administrators with database access can alter audit rows. Ship logs off-host for stronger evidence. |
| Information disclosure | Malicious HTML mail runs script or loads a tracker | Message bodies rendered in a sandboxed iframe under a strict CSP; scripts and form submission blocked; remote images blocked by default; remote content proxied through Gofer after opt-in | A browser vulnerability or an approved remote resource can still leak metadata. |
| Information disclosure | SSRF through a mailbox, calendar URL, webhook, or JMAP discovery | Outbound connections reject private, loopback, link-local, and reserved addresses unless an administrator approves the destination; redirects are rechecked | DNS rebinding and provider-side redirects remain a cat-and-mouse problem. Keep the allowlist narrow. |
| Information disclosure | Secrets appear in logs or error pages | Credentials are not rendered; authentication failures use generic messages; provider errors are redacted where possible | Debug/pprof must stay unavailable on public listeners. Review logs before sharing. |
| Information disclosure | Backup leaks credentials | Backups are created with restrictive permissions and contain the encrypted database; documentation warns that `secret.key` may be present | The archive itself is not encrypted by Gofer. Encrypt backups at rest and restrict access. |
| Denial of service | Login or password-hash flooding | Throttling, bounded request bodies, timeouts, and single-process SQLite write serialization | A large botnet can still exhaust CPU. Put Gofer behind a reverse proxy with connection limits. |
| Denial of service | Archive bomb or restore disk exhaustion | Manifest limit of 10 MiB, entry limit of 1,000,000, total extracted bytes limit of 100 GiB, per-entry size checks while copying | Large legitimate archives above 100 GiB are rejected. Raise the limit deliberately if needed. |
| Denial of service | AI provider hangs or returns errors | Per-job context deadlines, lease and retry policy, bounded concurrency, dead-letter state, circuit behavior in provider clients | A permanently failing provider leaves jobs retrying until the dead-letter policy applies. |
| Elevation of privilege | User reaches administrator routes | Separate admin authentication surface, role checks, step-up verification for sensitive actions, 404 for non-admin users | A compromised administrator account has broad control by design. Require MFA and review security events. |
| Elevation of privilege | Webhook or AI response triggers a privileged action | Automation is draft-first and validated against policy; webhook signatures verified; no automatic send | A user who approves a malicious draft can still send it. Keep the draft review step. |

## ASVS mapping

This is a control mapping, not a certification. The checklist below reflects the OWASP Application Security Verification Standard 5.0 structure.

| ASVS area | Coverage | Evidence and gaps |
| --- | --- | --- |
| V1 Encoding and sanitization | HTML mail sanitized; template auto-escaping; CSP for message iframes; archive paths rejected when absolute or traversing | Unit tests in `internal/mail/message`, `internal/views`, `internal/backup`. Browser-level XSS testing is not automated. |
| V2 Validation and business logic | Bounded request bodies, typed form parsing, ownership checks, state transitions for jobs and drafts, path and archive validation | Handler and worker tests. Fuzz tests cover selected parsers; archive parsing now has adversarial tests but not a continuous fuzz target. |
| V3 Web frontend | CSRF tokens, `HttpOnly` and `SameSite` cookies, sandboxed mail rendering, no inline scripts in message bodies | Handler/auth tests. Accessibility and CSP behavior need browser verification. |
| V4 API and web services | Authentication required by default, per-object authorization, JSON and form validation, signed webhook delivery | Handler tests and `internal/routing` tests. |
| V5 File handling | Attachment storage, signed attachment links, archive entry type checks, size and count limits, temp-file restore | Backup tests; attachment tests. Antivirus scanning is not built in. |
| V6 Authentication | Argon2id, password blocklist, throttling, MFA, recovery codes, OIDC, passkeys | `internal/auth` tests. Independent review not performed. |
| V7 Session management | Random session tokens, revocation, session listing, secure cookie settings, CSRF binding | `internal/auth` session tests. |
| V8 Authorization | User and administrator separation, per-user databases in managed mode, cross-user 404 behavior | Handler/server ownership tests. |
| V9 Self-contained tokens | Short-lived OAuth state and challenge values; no untrusted JWT accepted without signature and issuer checks | OAuth and OIDC tests. |
| V10 OAuth and OIDC | State, nonce, PKCE where supported, provider issuer and audience checks, separate mailbox and login clients | Provider callback tests. Provider-side configuration remains operator responsibility. |
| V11 Cryptography | AES-256-GCM with per-record associated data; Argon2id; SHA-256 for integrity; random keys | Crypto and storage tests. Key rotation is manual and must preserve old data. |
| V12 Secure communication | HTTPS required for public base URLs; plain HTTP allowed only for explicit private-network development | Config tests and documentation. TLS termination is delegated to a reverse proxy. |
| V13 Configuration | Loopback listener by default, explicit client-network allowlist, no debug endpoints on the public mux, documented secrets | Config and HTTP guard tests. Operators can still misconfigure open mode. |
| V14 Data protection | Encrypted credentials, per-user isolation, data-directory guidance, backup encryption left to the operator | Storage and backup tests. No field-level encryption for message bodies at rest. |
| V15 Secure coding | `go vet`, tests, dependency pinning, no dynamic SQL built from user paths, bounded goroutines and queues | CI runs vet and package tests on three operating systems; a dedicated workflow runs `govulncheck` on Go changes and weekly. No `staticcheck` gate yet. |
| V16 Logging and monitoring | Authentication and security events, structured logs, redaction of credential values, health and readiness endpoints | Event tests. No built-in SIEM integration. |
| V17 WebRTC | Not applicable | No WebRTC features. |

## Automated security assessment

The repository currently provides:

- `go test` coverage for authentication, authorization, CSRF, rendering, archive handling, webhook signature validation, and provider retry behavior.
- `go vet ./...` in CI on Linux, macOS, and Windows.
- `govulncheck` in [security.yml](../.github/workflows/security.yml) on Go changes, on every relevant pull request, and weekly.
- Adversarial backup tests for traversal, duplicate entries, unsafe database paths, and oversized manifests.
- Explicit rejection of symlinks and special files during restore.
- Integration-style CLI tests for backup, verification, restore, forced restore, and schema-version refusal.

Not yet performed:

- An independent third-party penetration test.
- A browser-driven XSS and CSP assessment against every rendered page.
- A `staticcheck` release gate.
- A restore drill against a production-sized data set.
- External review of OAuth provider configuration and reverse-proxy TLS settings.

A `govulncheck` run on 2026-10-25 found 19 reachable findings from `golang.org/x/net` v0.54.0 and the Go 1.27.1 standard library. The module now requires Go 1.27.3 and `golang.org/x/net` v0.62.0; a re-scan reported no reachable vulnerabilities and one vulnerability in a required module that the code does not call.

These gaps must stay visible in release notes and must not be described as passed.

## Penetration test status

No external penetration test has been commissioned for this fork. The automated assessment above is reproducible from the repository, but it does not replace adversarial human testing. Before claiming Gate G8 security completion, commission a test that covers at least:

1. Authentication bypass and session fixation in personal and managed modes.
2. Cross-user object access in every API and partial-render route.
3. Stored XSS through mail HTML, SVG, vCard, iCalendar, signatures, and AI output.
4. SSRF through account discovery, CalDAV URLs, webhook URLs, avatar fetchers, and remote images.
5. CSRF and clickjacking on state-changing forms and HTMX partials.
6. Backup and restore with malicious archives and hostile filesystem layouts.
7. Admin privilege boundaries and step-up authentication.
8. Rate-limit and denial-of-service behavior on login, search, sync, and AI endpoints.
9. Secret leakage in logs, metrics, error pages, and panic output.
10. Container and Helm defaults, including mounted secrets and network policy.

## Risk register

| Risk | Likelihood | Impact | Treatment |
| --- | --- | --- | --- |
| Open mode reachable from a network | Medium | High | Documented loopback-only default; warning in setup. Operators must choose personal or managed mode for remote access. |
| Data directory readable by another local user | Medium | High | Run as a dedicated user, use full-disk encryption, and keep backups encrypted. |
| Backup stolen | Low to medium | High | Encrypt archives at rest, restrict storage credentials, and rotate `secret.key` only with a planned re-encryption process. |
| AI provider retains message content | Medium | Medium | Use providers with acceptable retention terms, minimize prompt content, and document the data flow. |
| Dependency compromise | Low | High | Pin versions, review updates, and add `govulncheck` to CI. |
| Disk exhaustion during restore | Low | Medium | Preflight free space, keep safety backups off the target volume, and use the documented limits. |
| Misconfigured reverse proxy | Medium | High | Follow the setup guide, force HTTPS, set trusted proxy CIDRs, and verify forwarded headers. |

## Review cadence

Review this model when any of the following changes:

- A new provider, AI connector, webhook type, or public route is added.
- Authentication, session, CSRF, or authorization code changes.
- The data-directory layout, encryption format, or backup format changes.
- A new deployment target is supported.
- A security incident or penetration-test finding is closed.

At minimum, review it before each minor release and record the review date in the release notes.
