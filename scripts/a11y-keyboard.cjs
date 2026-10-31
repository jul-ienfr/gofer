#!/usr/bin/env node
const { chromium } = require('playwright');

const base = (process.env.GOFER_BASE_URL || 'http://127.0.0.1:8090').replace(/\/+$/, '');
const results = [];

function check(name, ok, detail) {
  results.push({ name, ok: Boolean(ok), detail: detail || '' });
}

async function active(page) {
  return page.evaluate(() => {
    const el = document.activeElement;
    return {
      tag: el?.tagName || '',
      id: el?.id || '',
      text: (el?.innerText || el?.textContent || '').trim().slice(0, 80),
      inDialog: Boolean(el?.closest?.('#compose-dialog dialog')),
    };
  });
}

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
    await page.goto(`${base}/`, { waitUntil: 'load' });

    await page.keyboard.press('Tab');
    const skip = await active(page);
    const skipVisible = await page.evaluate(() => {
      const link = document.querySelector('a[href="#main-content"]');
      if (!link) return false;
      const box = link.getBoundingClientRect();
      return box.width > 0 && box.height > 0;
    });
    check('first Tab reaches the skip link', skip.text === 'Skip to content', JSON.stringify(skip));
    check('focused skip link is visible', skipVisible);

    await page.keyboard.press('Enter');
    await page.waitForTimeout(150);
    const main = await active(page);
    check('skip link moves focus to main content', main.id === 'main-content', JSON.stringify(main));

    await page.locator('#sidebar-compose-btn').click();
    await page.waitForFunction(() => {
      const dialog = document.querySelector('#compose-dialog dialog');
      return dialog?.open === true;
    });
    await page.waitForTimeout(250);
    const opened = await active(page);
    check('compose dialog takes focus inside itself', opened.inDialog, JSON.stringify(opened));

    let escaped = false;
    for (let index = 0; index < 12; index += 1) {
      await page.keyboard.press('Tab');
      const current = await active(page);
      if (!current.inDialog) {
        escaped = true;
        check('Tab stays inside compose dialog', false, `after ${index + 1} tabs: ${JSON.stringify(current)}`);
        break;
      }
    }
    if (!escaped) check('Tab stays inside compose dialog', true);

    await page.keyboard.press('Escape');
    await page.waitForTimeout(450);
    const closed = await page.evaluate(() => {
      const dialog = document.querySelector('#compose-dialog dialog');
      return dialog?.open !== true;
    });
    check('Escape closes compose dialog', closed);
    const returned = await active(page);
    check('focus returns to the compose trigger', returned.id === 'sidebar-compose-btn', JSON.stringify(returned));
    check('compose trigger is enabled after closing', await page.locator('#sidebar-compose-btn').isEnabled());

    await page.locator('#sidebar-compose-btn').click();
    await page.waitForFunction(() => document.querySelector('#compose-dialog dialog')?.open === true);
    await page.keyboard.press('Escape');
    await page.waitForTimeout(450);
    check(
      'compose can be reopened after Escape',
      await page.evaluate(() => document.querySelector('#compose-dialog dialog')?.open !== true) &&
        (await page.locator('#sidebar-compose-btn').isEnabled()),
    );

    const positiveTabindex = await page.evaluate(() =>
      Array.from(document.querySelectorAll('[tabindex]')).filter((el) => Number(el.getAttribute('tabindex')) > 0).length,
    );
    check('no positive tabindex values', positiveTabindex === 0, `${positiveTabindex} found`);
  } finally {
    await browser.close();
  }

  const failed = results.filter((result) => !result.ok);
  for (const result of results) {
    console.log(`${result.ok ? 'PASS' : 'FAIL'} ${result.name}${result.detail ? ` — ${result.detail}` : ''}`);
  }
  if (failed.length > 0) {
    console.error(`${failed.length} keyboard check(s) failed`);
    process.exit(1);
  }
  console.log(`${results.length} keyboard checks passed`);
})().catch((error) => {
  console.error(error);
  process.exit(1);
});
