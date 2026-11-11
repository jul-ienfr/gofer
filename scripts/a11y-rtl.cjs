const { chromium } = require('playwright');

const base = process.env.GOFER_BASE_URL || 'http://127.0.0.1:8090';
const paths = [
  '/', '/contacts', '/calendar', '/settings',
  '/settings/accounts', '/settings/sync', '/settings/rules',
  '/settings/operations', '/settings/contacts', '/settings/appearance',
  '/settings/regional', '/settings/compose-display', '/settings/ai',
  '/settings/security', '/settings/advanced', '/settings/export-import',
  '/settings/about',
];
const spacing = `
  * { line-height: 1.5 !important; letter-spacing: 0.12em !important; word-spacing: 0.16em !important; }
  p { margin-bottom: 2em !important; }
`;

const failures = [];
const fail = (message) => failures.push(message);

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    // Reflow every primary page under a forced right-to-left root.
    for (const viewport of [{ width: 1280, height: 900 }, { width: 320, height: 800 }]) {
      const page = await browser.newPage({ viewport });
      for (const path of paths) {
        await page.goto(base + path, { waitUntil: 'load' });
        await page.evaluate(() => { document.documentElement.dir = 'rtl'; });
        if (viewport.width === 320) await page.addStyleTag({ content: spacing });
        const size = await page.evaluate(() => ({
          client: document.documentElement.clientWidth,
          scroll: document.documentElement.scrollWidth,
        }));
        if (size.scroll > size.client + 1) {
          fail(`${viewport.width}px ${path}: horizontal overflow ${size.scroll} > ${size.client}`);
        }
      }
      await page.close();
    }

    const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });
    await page.goto(base + '/', { waitUntil: 'load' });
    await page.evaluate(() => { document.documentElement.dir = 'rtl'; });

    const skip = await page.evaluate(() => {
      const link = document.querySelector('.skip-link');
      if (!link) return { missing: true };
      link.focus();
      const rect = link.getBoundingClientRect();
      return { width: rect.width, right: rect.right, client: document.documentElement.clientWidth };
    });
    if (skip.missing) fail('skip link is missing');
    else if (skip.width < 50 || skip.right < skip.client - 40) {
      fail(`skip link is not at the RTL reading edge (width=${skip.width}, right=${skip.right})`);
    }

    const search = await page.evaluate(() => {
      const input = document.querySelector('[data-mail-search-input]');
      if (!input) return { missing: true };
      const icon = input.parentElement.querySelector('svg[data-lucide="search"]');
      if (!icon) return { missingIcon: true };
      const i = getComputedStyle(icon);
      const s = getComputedStyle(input);
      return {
        iconRight: parseFloat(i.right),
        inputLeft: parseFloat(s.paddingLeft),
        inputRight: parseFloat(s.paddingRight),
      };
    });
    if (search.missing || search.missingIcon) fail('mail search field or its icon is missing');
    else if (search.iconRight > 20 || search.inputRight <= search.inputLeft) {
      fail(`mail search is not at the RTL reading edge (${JSON.stringify(search)})`);
    }

    const icons = await page.evaluate(() => {
      const probe = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
      probe.setAttribute('data-lucide', 'arrow-left');
      document.body.append(probe);
      const mirrored = getComputedStyle(probe).transform;
      probe.setAttribute('data-lucide', 'arrow-up');
      const upright = getComputedStyle(probe).transform;
      probe.remove();
      return { mirrored, upright };
    });
    if (icons.mirrored === 'none') fail('directional icons do not mirror under dir="rtl"');
    if (icons.upright !== 'none') fail('vertical icons mirror under dir="rtl"');

    const tristate = await page.evaluate(() => {
      const probe = document.createElement('div');
      probe.className = 'mail-filter-tristate';
      probe.dataset.mailTristateValue = '';
      const indicator = document.createElement('div');
      indicator.className = 'mail-filter-tristate-indicator';
      probe.append(indicator);
      document.body.append(probe);
      const transform = getComputedStyle(indicator).transform;
      probe.remove();
      return transform;
    });
    if (tristate === 'none') fail('three-state filter indicator does not follow the RTL segments');

    const center = (rect) => rect.x + rect.width / 2;
    await page.goto(base + '/contacts', { waitUntil: 'load' });
    const contactsToggle = await page.evaluate(() => {
      document.documentElement.dir = 'rtl';
      const group = document.querySelector('[data-contact-list-view-toggle]');
      if (!group || typeof syncListViewToggle !== 'function') return { missing: true };
      const indicator = group.querySelector('[data-contact-list-view-indicator]');
      const active = group.querySelector('[aria-pressed="true"]');
      if (!indicator || !active) return { missing: true };
      indicator.style.transition = 'none';
      syncListViewToggle(active.dataset.contactListViewButton);
      const i = indicator.getBoundingClientRect();
      const a = active.getBoundingClientRect();
      return { i: { x: i.x, width: i.width }, a: { x: a.x, width: a.width } };
    });
    if (contactsToggle.missing) fail('contacts Cards/Table switch is missing');
    else if (Math.abs(center(contactsToggle.i) - center(contactsToggle.a)) > 1) {
      fail(`contacts Cards/Table indicator does not follow RTL (${JSON.stringify(contactsToggle)})`);
    }

    await page.goto(base + '/calendar', { waitUntil: 'load' });
    const calendarToggle = await page.evaluate(() => {
      document.documentElement.dir = 'rtl';
      const group = document.querySelector('[data-calendar-view-nav]');
      if (!group || typeof setCalendarViewSwitch !== 'function') return { missing: true };
      const indicator = group.querySelector('[data-calendar-view-indicator]');
      const active = group.querySelector('[data-calendar-view-switch="month"]');
      if (!indicator || !active) return { missing: true };
      indicator.style.transition = 'none';
      setCalendarViewSwitch('month');
      const i = indicator.getBoundingClientRect();
      const a = active.getBoundingClientRect();
      return { i: { x: i.x, width: i.width }, a: { x: a.x, width: a.width } };
    });
    if (calendarToggle.missing) fail('calendar Month/Week switch is missing');
    else if (Math.abs(center(calendarToggle.i) - center(calendarToggle.a)) > 1) {
      fail(`calendar Month/Week indicator does not follow RTL (${JSON.stringify(calendarToggle)})`);
    }

    await page.goto(base + '/settings/advanced', { waitUntil: 'load' });
    const switchThumb = await page.evaluate(() => {
      document.documentElement.dir = 'rtl';
      const input = Array.from(document.querySelectorAll('input[role="switch"]')).find((node) => {
        const track = node.nextElementSibling;
        return track && track.getClientRects().length;
      });
      if (!input) return { missing: true };
      const track = input.nextElementSibling;
      const noTransition = document.createElement('style');
      noTransition.textContent = 'input[role="switch"] + .gofer-switch-track::after { transition: none !important; }';
      document.head.append(noTransition);
      const thumb = () => {
        const style = getComputedStyle(track, '::after');
        return { right: style.right, translate: style.translate };
      };
      input.checked = false;
      const off = thumb();
      input.checked = true;
      const on = thumb();
      return { off, on };
    });
    if (switchThumb.missing) fail('settings switch is missing');
    else if (switchThumb.off.right !== '0px' || switchThumb.on.right !== '0px' ||
      switchThumb.on.translate === 'none' || !switchThumb.on.translate.startsWith('-')) {
      fail(`switch thumb does not travel toward the RTL reading edge (${JSON.stringify(switchThumb)})`);
    }

    const themeBase = await page.evaluate(() => {
      document.documentElement.dir = 'rtl';
      const fieldset = document.createElement('fieldset');
      fieldset.setAttribute('data-custom-theme-choice', 'mode');
      fieldset.style.cssText = 'position:relative;width:300px';
      const indicator = document.createElement('div');
      indicator.setAttribute('data-custom-theme-base-indicator', '');
      indicator.style.cssText = 'position:absolute;left:2px;top:2px;bottom:2px;width:calc((100% - 6px)/2);transition:none';
      const dark = document.createElement('input');
      dark.type = 'radio';
      dark.name = 'rtl_theme_probe';
      dark.value = 'dark';
      dark.checked = true;
      fieldset.append(indicator, dark);
      document.body.append(fieldset);
      const translateX = (value) => {
        const match = value.match(/matrix\(([^)]+)\)/);
        return match ? parseFloat(match[1].split(',')[4]) : NaN;
      };
      const checked = translateX(getComputedStyle(indicator).transform);
      dark.checked = false;
      const unchecked = translateX(getComputedStyle(indicator).transform);
      fieldset.remove();
      return { checked, unchecked };
    });
    if (themeBase.checked !== 0 || !(themeBase.unchecked > 0)) {
      fail(`theme base indicator does not follow RTL options (${JSON.stringify(themeBase)})`);
    }
    await page.close();
  } finally {
    await browser.close();
  }

  if (failures.length) {
    for (const message of failures) console.error(`RTL: ${message}`);
    process.exit(1);
  }
  console.log(`RTL layout passed: ${paths.length} paths at 1280x900 and 320x800, skip link, search, icons, filter, view switches, theme base, toggle thumb.`);
})().catch((error) => {
  console.error(error);
  process.exit(1);
});
