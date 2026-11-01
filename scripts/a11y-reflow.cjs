#!/usr/bin/env node
const { chromium } = require('playwright');

const base = (process.env.GOFER_BASE_URL || 'http://127.0.0.1:8090').replace(/\/+$/, '');
const textSpacing = process.argv.includes('--text-spacing') || process.env.GOFER_TEXT_SPACING === '1';
const textSpacingCSS = `
  * { line-height: 1.5 !important; letter-spacing: 0.12em !important; word-spacing: 0.16em !important; }
  p { margin-bottom: 2em !important; }
`;
const paths = (process.env.GOFER_REFLOW_PATHS || [
  '/', '/contacts', '/calendar', '/settings',
  '/settings/accounts', '/settings/sync', '/settings/rules',
  '/settings/operations', '/settings/contacts', '/settings/appearance',
  '/settings/regional', '/settings/compose-display', '/settings/ai',
  '/settings/security', '/settings/advanced', '/settings/export-import',
  '/settings/about',
].join(',')).split(',').map((path) => path.trim()).filter(Boolean);

(async () => {
  const browser = await chromium.launch({ headless: true });
  let failures = 0;
  try {
    const page = await browser.newPage({ viewport: { width: 320, height: 800 } });
    for (const path of paths) {
      await page.goto(base + path, { waitUntil: 'load' });
      if (textSpacing) await page.addStyleTag({ content: textSpacingCSS });
      await page.waitForTimeout(150);
      const result = await page.evaluate(() => {
        const root = document.documentElement;
        return {
          clientWidth: root.clientWidth,
          scrollWidth: root.scrollWidth,
          title: document.title,
        };
      });
      const ok = result.scrollWidth <= result.clientWidth + 1;
      if (!ok) failures += 1;
      console.log(`${ok ? 'PASS' : 'FAIL'} ${path} client=${result.clientWidth} scroll=${result.scrollWidth} — ${result.title}`);
    }
  } finally {
    await browser.close();
  }
  const label = textSpacing ? 'reflow checks with increased text spacing' : 'reflow checks';
  if (failures > 0) {
    console.error(`${failures} ${label} failed at 320 CSS pixels`);
    process.exit(1);
  }
  console.log(`${paths.length} ${label} passed at 320 CSS pixels`);
})().catch((error) => {
  console.error(error);
  process.exit(1);
});
