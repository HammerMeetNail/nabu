import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {resolve} from 'node:path';

const manifest=JSON.parse(readFileSync(resolve('web/static/manifest.webmanifest'),'utf8'));

// /static/... in the manifest is web/static/... in the repository.
function file(src) {
  assert.match(src,/^\/static\//,src);
  return resolve('web'+src);
}

// A PNG's width and height are big-endian at bytes 16 and 20.
function pngSize(src) {
  const data=readFileSync(file(src));
  assert.equal(data.toString('latin1',1,4),'PNG',src);
  return `${data.readUInt32BE(16)}x${data.readUInt32BE(20)}`;
}

test('every manifest icon exists at the size it declares',()=>{
  const icons=[...manifest.icons,...manifest.shortcuts.flatMap(s=>s.icons||[])];
  for(const icon of icons) {
    if(icon.type==='image/png') assert.equal(pngSize(icon.src),icon.sizes,icon.src);
    else readFileSync(file(icon.src));
  }
});

test('Android gets maskable icons at 192 and 512 and a monochrome one',()=>{
  const sizes=purpose=>manifest.icons.filter(i=>i.purpose===purpose).map(i=>i.sizes).sort();
  assert.deepEqual(sizes('maskable'),['192x192','512x512']);
  assert.deepEqual(sizes('monochrome'),['512x512']);
});

test('each shortcut has its own icon',()=>{
  const srcs=manifest.shortcuts.map(s=>s.icons?.[0]?.src);
  assert.equal(new Set(srcs).size,manifest.shortcuts.length,srcs.join(', '));
});

test('the push badge the service worker names exists',()=>{
  const worker=readFileSync(resolve('web/static/service-worker.js'),'utf8');
  const badge=worker.match(/badge:"([^"]+)"/)?.[1];
  assert.ok(badge,'service worker sets a badge');
  assert.equal(pngSize(badge),'96x96');
});
