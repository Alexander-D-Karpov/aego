'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

// Exercise the actual search, render and keyboard handlers, not a copy of them.
const source = fs.readFileSync(path.join(__dirname, '../gendocs/site/static/site.js'), 'utf8');
const start = source.indexOf('    function escapeHTML(s) {');
const end = source.indexOf('    document.addEventListener("keydown", (e) => {', start);
assert(start >= 0 && end > start);
const searchCode = source.slice(start, end);

function harness(items, root = '') {
    const input = { value: 'search', listeners: {}, addEventListener(name, fn) { this.listeners[name] = fn; } };
    const results = { hidden: true, innerHTML: '' };
    const location = { href: 'original' };
    const context = { root, assetV: 'test', ru: false, SECTIONS: {reference: 'Reference'}, input, results, location,
        active: 0, shown: [], indexRequest: null,
        document: {querySelector: () => ({content: 'search.json'})},
        fetch: async () => ({ok: true, json: async () => items}),
        AegoDocsSearch: {rank: index => index.slice(0, 30)},
    };
    vm.runInNewContext(searchCode, context);
    return {input, results, location};
}

const item = (url, icon = 'help.book') => ({u: url, ic: icon, t: 'Title', k: 'page', c: 'reference'});

test('search escapes link and icon attributes as well as visible text', async () => {
    const h = harness([{...item('ref/article#anchor" onmouseover="run()', 'file" onload="run()'),
        t: '<img src=x onerror=run()>'}], '../');
    await h.input.listeners.input();
    assert(!h.results.innerHTML.includes('" onmouseover="'));
    assert(!h.results.innerHTML.includes('" onload="'));
    assert(!h.results.innerHTML.includes('<img src=x'));
    assert(h.results.innerHTML.includes('&lt;img'));
    assert(h.results.innerHTML.includes('ref/article#anchor&amp;') || h.results.innerHTML.includes('ref/article#anchor&quot;'));
});

test('unsafe search URLs cannot be clicked or opened with Enter', async () => {
    const malicious = ['javascript:alert(1)', ' JAVASCRIPT:alert(1)', '//evil.invalid/x',
        'https://evil.invalid/x', '\\evil.invalid\\x', 'ref/one\nother'];
    const h = harness([...malicious.map(url => item(url)), item('ref/good#anchor')], '../');
    await h.input.listeners.input();
    assert(!h.results.innerHTML.includes('javascript:'));
    assert(!h.results.innerHTML.includes('evil.invalid'));
    h.input.listeners.keydown({key: 'Enter'});
    assert.equal(h.location.href, '../ref/good#anchor');
});

test('valid Unicode and ordinary relative URLs remain navigable', async () => {
    const h = harness([item('ru/guide#Пример с пробелом')]);
    await h.input.listeners.input();
    assert(h.results.innerHTML.includes('ru/guide#Пример с пробелом'));
    h.input.listeners.keydown({key: 'Enter'});
    assert.equal(h.location.href, 'ru/guide#Пример с пробелом');
});

test('invalid URLs do not consume the search result limit', async () => {
    const h = harness([...Array.from({length: 35}, () => item('javascript:alert(1)')), item('ref/good')]);
    await h.input.listeners.input();
    h.input.listeners.keydown({key: 'Enter'});
    assert.equal(h.location.href, 'ref/good');
});
