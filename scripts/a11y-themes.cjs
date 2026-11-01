#!/usr/bin/env node
const { chromium } = require('playwright');
const AxeBuilder = require('@axe-core/playwright').default;

const base = (process.env.GOFER_BASE_URL || 'http://127.0.0.1:8090').replace(/\/+$/, '');
const looks = ['classic', 'minimal', 'express', 'wolf'];
const modes = ['light', 'dark'];
const paths = (process.env.GOFER_THEME_PATHS || [
  '/', '/contacts', '/calendar', '/settings',
  '/settings/accounts', '/settings/sync', '/settings/rules',
  '/settings/operations', '/settings/contacts', '/settings/appearance',
  '/settings/regional', '/settings/compose-display', '/settings/ai',
  '/settings/security', '/settings/advanced', '/settings/export-import',
  '/settings/about',
].join(','))
  .split(',')
  .map((path) => path.trim())
  .filter(Boolean);

(async () => {
  const browser = await chromium.launch({ headless: true });
  let failures = 0;
  try {
    const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
    const page = await context.newPage();
    for (const look of looks) {
      for (const mode of modes) {
        for (const path of paths) {
          await page.goto(base + path, { waitUntil: 'load' });
          await page.evaluate(({ look, mode }) => {
            document.documentElement.setAttribute('data-theme', look);
            document.documentElement.classList.toggle('dark', mode === 'dark');
          }, { look, mode });
          await page.waitForTimeout(700);
          const results = await new AxeBuilder({ page })
            .withTags(['wcag2a', 'wcag2aa', 'wcag21aa', 'wcag22aa'])
            .analyze();
          const violations = results.violations;
          if (violations.length) failures += 1;
          console.log(`${violations.length ? 'FAIL' : 'PASS'} ${look}/${mode} ${path}: ${violations.length} violation(s)`);
          for (const violation of violations) {
            console.log(`  [${violation.impact}] ${violation.id}: ${violation.help} (${violation.nodes.length})`);
            for (const node of violation.nodes) {
              console.log(`    ${node.target.join(' ')} :: ${node.html.slice(0, 160)} :: ${node.failureSummary}`);
            }
          }
        }
      }
    }
  } finally {
    await browser.close();
  }
  if (failures > 0) {
    console.error(`${failures} theme audit(s) failed`);
    process.exit(1);
  }
  console.log(`${looks.length * modes.length * paths.length} theme audits passed`);
})().catch((error) => {
  console.error(error);
  process.exit(1);
});
