#!/usr/bin/env node
// ui-shots: screenshot the studio in its main states, for before/after review. Not part of CI.
//
// Needs Chrome and puppeteer-core installed OUTSIDE the repo (the repo has no package.json on purpose):
//   mkdir -p /tmp/ac-shots && cd /tmp/ac-shots && npm init -y >/dev/null && npm i puppeteer-core@22
// Then, with a studio running on PORT (default 47400), for example `agent-canvas serve --dev --sample --project /tmp/ac-sample`:
//   NODE_PATH=/tmp/ac-shots/node_modules node scripts/ui-shots.mjs /tmp/ac-shots/out [PORT]
// It writes fit.png, entered-page.png, entered-screen.png, preview.png into the output directory and prints console errors.
// Set CHROME to the browser binary if it is not the macOS default.
import { createRequire } from 'module';
import fs from 'fs';
const require = createRequire(import.meta.url);
const puppeteer = require('puppeteer-core');

const [,, outDir = 'ui-shots', port = process.env.PORT || '47400'] = process.argv;
const chrome = process.env.CHROME || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';
fs.mkdirSync(outDir, { recursive: true });
const base = `http://127.0.0.1:${port}`;
const act = (type, payload, tab = 'web') => fetch(`${base}/api/act`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ type, payload, tab }) });
const wait = ms => new Promise(r => setTimeout(r, ms));

const browser = await puppeteer.launch({ executablePath: chrome, headless: 'new', args: ['--no-sandbox'] });
const page = await browser.newPage();
await page.setViewport({ width: 1600, height: 1000 });
const errors = [];
page.on('console', m => { if (m.type() === 'error') errors.push(m.text().slice(0, 200)); });
page.on('pageerror', e => errors.push('pageerror: ' + e.message));

const shot = async (name, ms = 1200) => { await wait(ms); await page.screenshot({ path: `${outDir}/${name}.png` }); console.log('wrote', `${outDir}/${name}.png`); };

await act('set_entered', { nodeId: '' });
await act('set_selection', { selection: null });
await act('set_canvas_view', { x: 0, y: 0, zoom: 0 });
await page.goto(base + '/', { waitUntil: 'domcontentloaded' });
await shot('fit', 2500);

const state = await (await fetch(`${base}/api/state`)).json();
const firstPage = state.project.nodes.find(n => n.kind === 'page');
const firstScreen = state.project.nodes.find(n => n.kind === 'screen');
if (firstPage) { await act('set_entered', { nodeId: firstPage.id }); await shot('entered-page'); }
if (firstScreen) { await act('set_entered', { nodeId: firstScreen.id }, 'story'); await shot('entered-screen'); }
if (firstPage) {
  await act('set_entered', { nodeId: firstPage.id });
  await act('set_preview', { on: true });
  await shot('preview');
  await act('set_preview', { on: false });
}
await act('set_entered', { nodeId: '' });
console.log('console errors:', errors.length ? errors : 'none');
await browser.close();
