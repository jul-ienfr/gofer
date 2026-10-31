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

Last automated checks in this repository on 2026-10-25 used an open-mode, empty data set for settings and a seeded 10,000-message fixture for mail, contacts, and Calendar:

- Structural checker: zero violations for `/`, `/contacts`, `/calendar`, `/settings`, `/login`, and `/setup`.
- Browser audit: zero violations for `/`, `/contacts`, `/calendar`, all 13 settings pages, and `/setup` at 1280×900 and 390×844, with light and dark color schemes.
- Seeded browser audit: zero violations for `/`, `/contacts`, and `/calendar` at desktop and mobile after fixing month-event target size.
- Keyboard audit: 10 checks pass on both the empty and seeded databases.
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

- The interface is English-only. There is no translation catalog, language switcher, or locale negotiation.
- Full pages declare `lang="en"`.
- Dates and times use the browser and Go formatting paths already present in the application; they have not been audited for every locale.
- Mail, contacts, and Calendar content is displayed in its original language, independent of the interface language.
- No right-to-left layout support has been implemented or tested.
- User-facing error strings are English and sometimes include Go error text.

Before claiming i18n support, add:

1. A message catalog with stable keys and pluralization.
2. A per-user language preference and `lang` attribute update.
3. Locale-aware date, time, number, and relative-time formatting.
4. RTL layout and icon mirroring tests.
5. Translation review for security-sensitive messages, especially sign-in, recovery, and deletion.
6. A test that every visible string comes from the catalog rather than a Go literal.

Until then, release notes should say "English UI, multilingual mail content" rather than "internationalized".

## Reporting issues

Include the page, browser, assistive technology, viewport or zoom level, and the steps to reproduce. Accessibility defects that block sign-in, reading mail, or account recovery are treated as release blockers.
