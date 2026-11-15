'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const test = require('node:test');

// Fault-injection at the presentation transport boundary. The separate HTTP
// GPU E2E gate exercises real WebGL2, shaders, particles and context restore.
function fixture() {
  const elements = new Map(), raf = [], intervals = [];
  function element(id) {
    if (elements.has(id)) return elements.get(id);
    const classes = new Set();
    const item = {value: id === 'gpu-mode' ? 'lit' : 1, checked: false,
      textContent: '', title: '', options: [{value:'pixels'}, {value:'lit'}],
      addEventListener() {}, classList: {
        add(name) { classes.add(name); },
        toggle(name, on) { if (on) classes.add(name); else classes.delete(name); },
        contains(name) { return classes.has(name); }
      }};
    elements.set(id, item); return item;
  }
  let current = {state:'ready', revision:1, sequence:1, mode:'lit', renderer:'transport-test'};
  let failure = 'publish';
  const api = {
    start() { return JSON.stringify({status:current}); },
    publish(sequence, revision) {
      if (failure === 'publish') return JSON.stringify({error:'Prepared pixels exceed the resource budget'});
      current = {...current, revision, sequence};
      return JSON.stringify({status:current});
    },
    configure() {
      if (failure === 'configure') return JSON.stringify({error:'Invalid preview parameters'});
      return JSON.stringify({status:current});
    },
    status() { return JSON.stringify({status:current}); }, close() {}
  };
  const context = {
    document:{getElementById:element, documentElement:{lang:'en'}, body:element('body')},
    requestAnimationFrame(fn) { raf.push(fn); }, setInterval(fn) { intervals.push(fn); },
    fetch:async () => ({ok:true, clone() { return this; }, arrayBuffer:async () => new ArrayBuffer(0)}),
    WebAssembly:{instantiateStreaming:async () => ({instance:{}})},
    Go:class { constructor() { this.importObject = {}; } run() { return Promise.resolve(); } },
    Uint8Array, JSON, Number, Promise
  };
  context.window = {aegoArtPreview:api, addEventListener() {}};
  vm.createContext(context);
  vm.runInContext(fs.readFileSync(path.join(__dirname, '../../web/gpu-preview.js'), 'utf8'), context);
  return {
    async update(revision = 2) {
      context.window.AegoGPUPreview.update({documentKey:'doc', revision, frame:'frame', gesture:false,
        api:{snapshotBytes() { return {handle:'doc', revision, bytes:new Uint8Array(1)}; }}});
      await raf.shift()();
    },
    poll() { intervals[0](); },
    failAt(value) { failure = value; },
    get message() { return element('gpu-status').textContent; },
    get failed() { return element('gpu-preview').classList.contains('gpu-failed'); }
  };
}

test('a rejected snapshot remains an error while the renderer reports its old ready frame', async () => {
  const f = fixture();
  await f.update();
  assert.equal(f.failed, true);
  assert.match(f.message, /resource budget/);
  for (let i = 0; i < 5; i++) f.poll();
  assert.equal(f.failed, true);
  assert.match(f.message, /resource budget/);
  f.failAt(''); await f.update();
  assert.equal(f.failed, false);
  assert.match(f.message, /Ready/);
});

test('a configuration failure clears only after successful reconfiguration', async () => {
  const f = fixture(); f.failAt('configure');
  await f.update(); f.poll();
  assert.equal(f.failed, true);
  assert.match(f.message, /parameters/);
  f.failAt(''); await f.update(); f.poll();
  assert.equal(f.failed, false);
  assert.match(f.message, /Ready/);
});
