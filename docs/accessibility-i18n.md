# Accessibility and internationalization

Gofer targets WCAG 2.2 AA for the application shell and primary mail, contacts, Calendar, settings, login, and setup flows. This document records the automated checks, the manual checks still required, and the current internationalization status.

## Automated check

`cmd/a11ycheck` fetches rendered pages and reports structural violations. It is dependency-free apart from Go and the packages already in `go.mod`.

```sh
go run ./cmd/a11ycheck \
  -base http://127.0.0.1:8090 \
  -path / -path /contacts -path /calendar -path /settings
```

Authenticated installations need a session cookie copied from the browser:

```sh
go run ./cmd/a11ycheck -base https://mail.example.com \
  -cookie 'gofer_session=<value>' \
  -path / -path /contacts -path /calendar -path /settings
```

PowerShell:

```powershell
$env:GOFER_COOKIE = 'gofer_session=<value>'
go run ./cmd/a11ycheck -base https://mail.example.com -path / -path /contacts -path /calendar -path /settings
```

The command exits non-zero when it finds a violation and can print JSON with `-json`. It checks:

- A non-empty document language and title.
- A `main` landmark and a skip link when the page has navigation.
- An accessible name for each visible link, button, and form control.
- `alt` attributes on images.
- Valid `aria-labelledby` references and no positive `tabindex`.
- Duplicate visible `id` values.

It does not evaluate color contrast, zoom, reflow, full keyboard order, screen-reader output, or reduced-motion behavior. Hidden dialogs and `<template>` content are skipped, so open the relevant dialog before a manual check. The separate keyboard audit covers focus trapping for the Compose dialog and its return-focus path.

### Browser audit

`scripts/a11y-browser.cjs` runs axe-core in headless Chromium against the rendered page. It checks the WCAG 2.0 A/AA, 2.1 AA, and 2.2 AA rule tags, including color contrast and target size.

```sh
npm install --no-save @tailwindcss/cli@4.3.3 playwright@1.64.1 @axe-core/playwright@4.11.0 axe-core@4.11.0
npx playwright install chromium
task generate
node scripts/a11y-browser.cjs \
  --base http://127.0.0.1:8090 \
  --path / --path /contacts --path /calendar --path /settings
```

Generate the assets first so the server serves the current CSS and component JavaScript. Without `task`, run the Tailwind and esbuild commands from the `generate` task manually.

Pass `--cookie 'gofer_session=...'` for an authenticated instance, `--viewport 390x844` for mobile, `--color-scheme dark` for dark mode, and `--json` for machine-readable output. The script exits non-zero on any violation.

### Keyboard audit

`scripts/a11y-keyboard.cjs` drives a real Chromium page and checks the paths axe cannot:

```sh
node scripts/a11y-keyboard.cjs
```

It sets `GOFER_BASE_URL` (default `http://127.0.0.1:8090`) and verifies:

- The first Tab reaches the visible skip link, and activating it focuses `main-content`.
- Compose moves focus into its modal dialog, Tab stays inside, Escape closes it, focus returns to the Compose button, and the button is enabled and can open Compose again.
- No positive `tabindex` values exist on the page.

The accessibility workflow runs this after the axe passes.

### Reflow audit

`scripts/a11y-reflow.cjs` loads the primary pages at a 320 CSS-pixel viewport and
checks that the document does not require horizontal scrolling:

```sh
GOFER_BASE_URL=http://127.0.0.1:8090 node scripts/a11y-reflow.cjs
```

It covers `/`, `/contacts`, `/calendar`, `/settings`, and all 13 settings pages.
Pass `--text-spacing` to inject the WCAG 1.4.12 spacing overrides (1.5 line
height, 0.12em letter spacing, 0.16em word spacing, and 2em paragraph spacing)
before checking for horizontal scrolling. The accessibility workflow runs both
modes after the keyboard checks.

### Reduced-motion audit

`scripts/a11y-motion.cjs` loads the primary pages with
`prefers-reduced-motion: reduce` and fails if a non-essential CSS animation is
still running:

```sh
GOFER_BASE_URL=http://127.0.0.1:8090 node scripts/a11y-motion.cjs
```

Essential loading spinners are exempt. The audit found that the fade-in and
pulse utilities ignored the preference; the stylesheet now disables them along
with the slide utilities.

### Theme audit

`scripts/a11y-themes.cjs` runs axe on all 17 primary and settings pages in every
shipped look (`classic`, `minimal`, `express`, `wolf`) and both color modes:

```sh
GOFER_BASE_URL=http://127.0.0.1:8090 node scripts/a11y-themes.cjs
```

The audit found and we fixed an undersized Express select-all checkbox, an
Express dark link that used the button blue as text, and the Calendar "Today"
badge that used the accent color as text on its own tint.

The latest automated checks on 2026-11-01 used an open-mode, empty data set; an earlier seeded 10,000-message fixture covered mail, contacts, and Calendar:

- Structural checker: zero violations for `/`, `/contacts`, `/calendar`, `/settings`, `/login`, and `/setup`.
- Browser audit: zero violations for `/`, `/contacts`, `/calendar`, all 13 settings pages, and `/setup` at 1280×900 and 390×844, with light and dark color schemes.
- Seeded browser audit: zero violations for `/`, `/contacts`, and `/calendar` at desktop and mobile after fixing month-event target size.
- Keyboard audit: 10 checks pass on both the empty and seeded databases.
- Reflow audit: 17 pages pass at 320 CSS pixels with no horizontal document scroll, including with WCAG 1.4.12 text-spacing overrides.
- Reduced-motion audit: 7 pages pass with no non-essential animation running; fade-in, slide, and pulse utilities are disabled under the preference.
- Theme audit: 136 checks pass (4 looks × 2 modes × 17 pages) with zero axe violations.
- Opened dialogs: zero violations in Compose, Filters, New event, Add Account, the IMAP/SMTP account wizard, and Contact actions.
- Password reveal controls expose `Show password` / `Hide password` and `aria-pressed`; verified in the IMAP account wizard.

The audits found and we fixed an ARIA attribute on the active-filter strip and filter tabs, unnamed recipient fields, 16–20 px selection controls, a link that relied on color alone, month-event buttons below the 24 px target size, the mail shortcut swallowing Enter on focused links and buttons, and the Compose button staying disabled after a dialog close. This is not a claim of full WCAG conformance. It does not replace the manual checks below.

## Manual WCAG 2.2 AA checklist

Run this before a release that changes layout or navigation, and at least once per release cycle.

### Keyboard and focus

- [ ] Tab through the page in a logical order; every control is reachable and operable.
- [ ] The skip link becomes visible on focus and moves focus to the main content.
- [ ] Focus is always visible and not clipped by sticky headers or overflow containers.
- [ ] Dialog and popover focus is trapped while open, Escape closes, and focus returns to the trigger.
- [ ] Drag-and-drop actions have a keyboard-accessible alternative.
- [ ] No keyboard trap exists in the mail reading iframe.

### Names, roles, and states

- [ ] Icon-only buttons announce a useful name, not an icon filename.
- [ ] Checkbox, radio, switch, and select states are announced.
- [ ] Validation errors are associated with the field and announced.
- [ ] Sort and filter state is reflected in `aria-sort`, `aria-pressed`, or an equivalent.
- [ ] Tables expose headers; the mail list has a usable reading order.
- [ ] Loading and sync state changes are announced without flooding the screen reader.

### Visual

- [ ] Body text reaches 4.5:1 contrast; large text reaches 3:1; controls and focus rings reach 3:1.
- [ ] Information is not conveyed by color alone.
- [ ] Text remains usable at 200% zoom and the layout reflows at 320 CSS pixels without two-dimensional scrolling.
- [ ] `prefers-reduced-motion` stops non-essential animation.
- [ ] Both light and dark themes pass, and so do the classic, minimal, express, and wolf looks.
- [ ] The skip link is hidden until focus and does not overlap controls when shown.

### Assistive technology

- [ ] VoiceOver on macOS or Safari with the primary mail, contacts, Calendar, login, and settings flows.
- [ ] NVDA on Windows with the same flows.
- [ ] Screen-reader mode does not read hidden mobile/desktop duplicate panes.
- [ ] Mail HTML remains readable with images blocked and styles reduced.

### Forms and errors

- [ ] Labels are programmatically associated, not just visually adjacent.
- [ ] Required fields are announced as required.
- [ ] Error summaries and field errors are reachable and descriptive.
- [ ] Password reveal and copy buttons announce their action and state.

## Internationalization

Current status:

- English (`en`) and French (`fr`) catalogs live in `internal/i18n`. English is the source catalog; a parity test requires every key in both catalogs and rejects empty values.
- Each user chooses the interface language in **Settings → Regional → Interface language**. The choice is stored as `language` in the user's UI settings and applied to authenticated pages through `<html lang>`. A missing or unknown stored value falls back to English.
- The unauthenticated login page uses the `gofer_lang` cookie set when a signed-in user saves the interface language, then the browser's `Accept-Language` header. The cookie is HTTP-only, SameSite=Lax, and expires after one year.
- The settings navigation, section headings, page title, Regional tab, the main login screen, the full MFA sign-in, enrollment, and recovery-repair flow, personal setup, the initial setup token page, and the complete invitation and password-reset flow, including completion messages, are translated. The rest of the interface, including the owner setup wizard and the security settings partials, is still English.
- Mail, contacts, and Calendar content is displayed in its original language, independent of the interface language.
- Dates and times use the browser and Go formatting paths already present in the application; they have not been audited for every locale.
- No right-to-left layout support has been implemented or tested.
- User-facing error strings are English and sometimes include Go error text.

To add a language:

1. Add the locale and a complete catalog map in `internal/i18n`; the parity test requires every English key.
2. Expose it in `Supported` and add its native name in `NativeName`.
3. Translate templates with `tr(uiSettings, "key")` instead of Go literals.

Before claiming full i18n support, still add:

1. Catalog coverage for every visible string, with a test that rejects user-facing Go literals.
2. Locale-aware date, time, number, and relative-time formatting.
3. RTL layout and icon mirroring tests.
4. Translation review for security-sensitive messages, especially sign-in, recovery, and deletion.

Until then, release notes should say "English and French settings UI, multilingual mail content" rather than "internationalized".

## Reporting issues

Include the page, browser, assistive technology, viewport or zoom level, and the steps to reproduce. Accessibility defects that block sign-in, reading mail, or account recovery are treated as release blockers.
