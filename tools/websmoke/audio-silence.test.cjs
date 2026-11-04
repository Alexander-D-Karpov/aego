// Exercise the real browser-audio gate with fake page state. These policy
// checks do not replace browser playback, nor do zero underruns prove audio.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const {test} = require('node:test');
const vm = require('node:vm');

const file = path.join(__dirname, 'audio.mjs');
const source = fs.readFileSync(file, 'utf8');
const start = source.indexOf('async function run(browser, label, headers, wantShared, depth = 0) {');
const end = source.indexOf('\nconst browser = await chromium.launch(', start);
if (start < 0 || end < 0) throw new Error('unexpected audio smoke source');
const code = source.slice(start, end);

async function evaluate(rms, isDemo, depth = 0) {
    const failures = [];
    const stats = {running:true, queued:2048, shared:false, rms, out:rms, muted:false,
        underruns:0, pumpUnderruns:0, targetBlocks:depth || 6, blockFrames:512, rate:48000, mixMaxUs:50};
    const page = {on(){}, goto:async()=>{}, close:async()=>{}, reload:async()=>{},
        mouse:{click:async()=>{}}, isHidden:async()=>false,
        click:async selector=>{if (selector === '#mute') {stats.muted=true;stats.out=0;}}};
    const scope = {
        fs:{existsSync:()=>true}, path, site:'/diagnostic-only', seconds:10, demoSeconds:10,
        sleep:async()=>{}, fail:s=>failures.push(s), console:{log(){}},
        serve:async()=>({address:()=>({port:1}),close(){}}),
        scriptWithoutGesture:async()=>({until:async()=>{}, js:async expression=>{
            if (expression==='globalThis.crossOriginIsolated === true') return false;
            if (expression.includes('const c = new AudioContext()')) return false;
            if (expression.startsWith('globalThis.aegoAudioBlocks =')) return undefined;
            if (expression==='globalThis.aegoAudio.start()') return '';
            if (expression==='globalThis.aegoAudio.stats()' || expression==='globalThis.aegoAudioStats()') return JSON.stringify(stats);
            if (expression==='globalThis.aegoAudio.stop()' || expression==='globalThis.aegoAudioMute(false)') return undefined;
            if (expression==="document.documentElement.dataset.sound === 'off'") return false;
            if (expression==='navigator.userActivation && navigator.userActivation.hasBeenActive') return false;
            if (expression==='globalThis.aegoGame?.mixStats?.() ?? null') return {underruns:0};
            throw new Error('unexpected test expression: '+expression);
        }}),
    };
    vm.createContext(scope);
    vm.runInContext(code,scope,{filename:file});
    const browser={newPage:async()=>page};
    if (isDemo) await scope.demo(browser,'demo',{},false);
    else await scope.run(browser,'messages',{},false,depth);
    return failures;
}

test('tone silence fails even when both underrun counters are zero',async()=>{
    assert((await evaluate(0,false)).some(x=>x.includes('rms 0')));
    assert((await evaluate(0,false,5)).some(x=>x.includes('rms 0')));
});
test('game silence fails even when both underrun counters are zero',async()=>{
    assert((await evaluate(0,true)).some(x=>x.includes('music is silent')));
});
test('audible samples and zero underruns do not fail the fake checks',async()=>{
    assert.deepEqual(await evaluate(0.354,false),[]);
    assert.deepEqual(await evaluate(0.09,true),[]);
});
