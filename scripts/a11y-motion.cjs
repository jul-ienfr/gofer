#!/usr/bin/env node
const { chromium } = require('playwright');

const base = (process.env.GOFER_BASE_URL || 'http://127.0.0.1:8090').replace(/\/+$/, '');
const paths = ['/', '/contacts', '/calendar', '/settings', '/settings/appearance', '/settings/ai', '/settings/about'];

(async () => {
  const browser = await chromium.launch({ headless: true });
  let failures = 0;
  try {
    const page = await browser.newPage({ viewport: { width: 1280, height: 900 }, reducedMotion: 'reduce' });
    for (const path of paths) {
      await page.goto(base + path, { waitUntil: 'load' });
      await page.waitForTimeout(300);
      const result = await page.evaluate(() => {
        const active = [];
        for (const el of document.querySelectorAll('body *')) {
          const style = getComputedStyle(el);
          if (style.display === 'none' || style.visibility === 'hidden') continue;
          if (el.classList.contains('animate-spin')) continue;
          const name = style.animationName;
          const duration = style.animationDuration;
          if (name && name !== 'none' && duration && duration !== '0s') {
            active.push({
              tag: el.tagName.toLowerCase(),
              id: el.id || '',
              cls: (el.className || '').toString().slice(0, 60),
              name,
              duration,
            });
          }
        }
        return { active: active.slice(0, 8), count: active.length };
      });
      const ok = result.count === 0;
      if (!ok) failures += 1;
      console.log(`${ok ? 'PASS' : 'FAIL'} ${path} active_animations=${result.count}`);
      for (const item of result.active) {
        console.log(`  ${item.tag}#${item.id}.${item.cls} ${item.name} ${item.duration}`);
      }
    }
  } finally {
    await browser.close();
  }
  if (failures > 0) {
    console.error(`${failures} reduced-motion check(s) failed`);
    process.exit(1);
  }
  console.log(`${paths.length} reduced-motion checks passed`);
})().catch((error) => {
  console.error(error);
  process.exit(1);
});
