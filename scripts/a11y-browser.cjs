#!/usr/bin/env node
const { chromium } = require('playwright');
const AxeBuilder = require('@axe-core/playwright').default;

function usage() {
  console.error('usage: node scripts/a11y-browser.cjs [--base URL] [--cookie VALUE] [--locale LOCALE] [--viewport WxH] [--color-scheme light|dark] [--path PATH]... [--json]');
}

const args = process.argv.slice(2);
let base = process.env.GOFER_BASE_URL || 'http://127.0.0.1:8090';
let cookie = process.env.GOFER_COOKIE || '';
let json = false;
let viewport = { width: 1280, height: 900 };
let colorScheme = 'light';
let locale = '';
const paths = [];

for (let index = 0; index < args.length; index += 1) {
  const arg = args[index];
  if (arg === '--base') {
    base = args[++index];
  } else if (arg === '--cookie') {
    cookie = args[++index];
  } else if (arg === '--locale') {
    locale = args[++index] || '';
  } else if (arg === '--viewport') {
    const match = /^(\d+)x(\d+)$/.exec(args[++index] || '');
    if (!match) {
      console.error('viewport must look like 1280x900');
      process.exit(2);
    }
    viewport = { width: Number(match[1]), height: Number(match[2]) };
  } else if (arg === '--color-scheme') {
    colorScheme = args[++index];
    if (colorScheme !== 'light' && colorScheme !== 'dark') {
      console.error('color scheme must be light or dark');
      process.exit(2);
    }
  } else if (arg === '--path') {
    paths.push(args[++index]);
  } else if (arg === '--json') {
    json = true;
  } else if (arg === '--help' || arg === '-h') {
    usage();
    process.exit(0);
  } else {
    console.error(`unknown argument: ${arg}`);
    usage();
    process.exit(2);
  }
}
if (paths.length === 0) {
  paths.push('/');
}
base = base.replace(/\/+$/, '');

(async () => {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    viewport,
    colorScheme,
    reducedMotion: 'reduce',
    ...(locale ? { locale } : {}),
  });
  if (cookie) {
    await context.setExtraHTTPHeaders({ Cookie: cookie });
  }
  const results = [];
  let failed = false;
  for (const path of paths) {
    const page = await context.newPage();
    const target = base + path;
    const result = { url: target, title: '', lang: '', violations: [] };
    try {
      const response = await page.goto(target, { waitUntil: 'domcontentloaded', timeout: 30000 });
      await page.waitForTimeout(750);
      result.title = await page.title();
      result.lang = await page.evaluate(() => document.documentElement.lang);
      if (locale && result.lang.toLowerCase() !== locale.toLowerCase()) {
        result.violations.push({
          id: 'locale-mismatch',
          impact: 'serious',
          help: `expected lang="${locale}", got lang="${result.lang}"`,
          nodes: [],
        });
      }
      const audit = await new AxeBuilder({ page })
        .withTags(['wcag2a', 'wcag2aa', 'wcag21aa', 'wcag22aa'])
        .analyze();
      result.violations = audit.violations.map((violation) => ({
        id: violation.id,
        impact: violation.impact,
        help: violation.help,
        nodes: violation.nodes.map((node) => ({
          target: node.target,
          html: node.html,
          summary: node.failureSummary,
        })),
      }));
      if (response && response.status() !== 200) {
        result.violations.push({
          id: 'http-status',
          impact: 'serious',
          help: `expected 200, got ${response.status()}`,
          nodes: [],
        });
      }
    } catch (error) {
      result.violations.push({
        id: 'audit-error',
        impact: 'serious',
        help: error.message,
        nodes: [],
      });
    } finally {
      await page.close();
    }
    if (result.violations.length > 0) {
      failed = true;
    }
    results.push(result);
  }
  await browser.close();

  if (json) {
    console.log(JSON.stringify(results, null, 2));
  } else {
    for (const result of results) {
      console.log(`${result.url}: ${result.violations.length} violation(s) — ${result.title}`);
      for (const violation of result.violations) {
        console.log(`  [${violation.impact}] ${violation.id}: ${violation.help}`);
        for (const node of violation.nodes.slice(0, 3)) {
          console.log(`    ${node.target.join(' ')}`);
          if (node.summary) {
            console.log(`      ${node.summary.split('\n')[0]}`);
          }
        }
      }
    }
  }
  process.exit(failed ? 1 : 0);
})().catch((error) => {
  console.error(error);
  process.exit(2);
});
