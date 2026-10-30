// Renders the PWA icons in web/static/icons from their SVG sources with
// Chromium, whose SVG rendering matches the browsers that show them
// (ImageMagick's own renderer drops strokes).
//
// Usage: node scripts/render-icons.cjs   (needs `pnpm install` for Playwright)
const { chromium } = require('@playwright/test');
const { readFile } = require('node:fs/promises');
const path = require('node:path');

const dir = path.join(__dirname, '..', 'web', 'static', 'icons');
const icons = [
  ['icon-monochrome.svg', 'icon-monochrome-512.png', 512],
  ['shortcut-feed.svg', 'shortcut-feed-192.png', 192],
  ['shortcut-log.svg', 'shortcut-log-192.png', 192],
  ['shortcut-activity.svg', 'shortcut-activity-192.png', 192],
  ['notification-badge.svg', 'notification-badge-96.png', 96],
];

(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage();
  for (const [svg, png, size] of icons) {
    const markup = await readFile(path.join(dir, svg), 'utf8');
    await page.setViewportSize({ width: size, height: size });
    await page.setContent(`<style>html,body{margin:0;background:transparent}svg{display:block;width:${size}px;height:${size}px}</style>${markup}`);
    await page.screenshot({ path: path.join(dir, png), omitBackground: true });
  }
  await browser.close();
})();
