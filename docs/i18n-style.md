# Translation style guide

This guide covers the JSON catalogs in `internal/i18n/locales`. English is the
source; French, Spanish, Brazilian Portuguese, and Simplified Chinese are fully
translated. A missing value falls back to English at runtime, but tests reject
that for AI keys and for every mapped server error.

## Keys and placeholders

- Translate values, never keys. Keep keys exactly as they appear in `en.json`.
- Keep every `{placeholder}` exactly, including braces and spelling. Examples:
  `{count}`, `{name}`, `{email}`, `{account}`, `{date}`, `{rule}`.
- Do not translate a placeholder or add a new one. The placeholder test rejects
  unknown names and missing English names.
- Named placeholders may move to fit the sentence. Placeholders inside a value
  must not be repeated unless the English value repeats them.
- Keep leading and trailing spaces when the value is a prefix or suffix, such
  as `mail.labels.create_prefix` and `mail.labels.create_suffix`.

## Plural forms

Counted keys have `.one` and `.other` forms and are rendered with `i18n.N` on
the server or `goferTn` in the browser.

| Locale | `.one` used for | Notes |
| --- | --- | --- |
| English | 1 | Other counts use `.other`. |
| French | 0 and 1 | French treats zero as singular. |
| Spanish | 1 | Zero uses `.other`. |
| Brazilian Portuguese | 0 and 1 | Portuguese treats zero as singular. |
| Simplified Chinese | 1 | Chinese has no grammatical plural; `.one` carries the natural singular wording. |

- `.other` must contain `{count}`. `.one` may omit it when the natural wording
  does not need the number, such as `每天` ("every day") or `Chaque jour`.
- Keep both forms even when they are identical, as in Chinese.
- Do not use a plural form for a value that is not a counted message.

## Dates, numbers, and units

- Date and time order, separators, digit grouping, decimal marks, and byte
  units come from catalog keys such as `date.pattern.*`, `number.group`,
  `number.decimal`, and `number.unit.*`. Do not hardcode them in templates or
  scripts.
- `scripts/test-i18n-dates.cjs` must agree with the Go formatter. Update both
  when a date or number pattern changes.
- Byte units are localized: French uses `o`, `ko`, `Mo`, `Go`; English and the
  other catalogs use `B`, `kB`, `MB`, `GB`.

## Tone and terminology

- French uses `vous`. Spanish uses informal `tú`. Brazilian Portuguese uses
  direct imperatives and normally omits `você`. Chinese is concise and avoids
  redundant pronouns.
- Keep product and protocol names as they are: Gofer, Jev, Gofer Sync, Web Push,
  Webhook, Spam, VIP, LLM, API, HTTPS, RFC 8058, token. Translate surrounding
  prose, not the name.
- Prefer the terminology already used elsewhere in the same catalog. Search the
  catalog before introducing a new word for "mailbox", "draft", "sender",
  "connector", or "routing".
- Server errors start in lowercase and have no trailing period, matching
  `server.error.*`. User-facing sentences and warnings use normal sentence
  punctuation.

## Security-sensitive messages

- Keep the meaning of sign-in, recovery, password, 2FA, deletion, phishing, and
  permission warnings. Do not soften "never", "cannot be undone", "requires
  review", or "is not allowed".
- Do not turn a warning into a suggestion. A phishing result must still say that
  automation does not run.
- Raw Go error details may follow a translated prefix; do not translate the raw
  detail in the catalog.
- Review these screens first: sign-in and MFA, password reset, account deletion,
  security activity, AI phishing/spam settings, and connector secrets.

## Before submitting a translation

1. Run `go test ./internal/i18n ./internal/views`.
2. Run `go test ./internal/handler -run 'TestServerText'`.
3. Run `node scripts/test-i18n-dates.cjs`.
4. Read the changed values in context, not only as a diff.
5. Check that buttons and labels still fit at 320 CSS pixels and at 200% zoom.
6. Have a native speaker review security-sensitive messages before release.

The automated tests cover key parity, placeholders, encoding, AI completeness,
server-error mappings, dates, numbers, byte units, and plural forms. They do not
judge whether a translation sounds natural.
