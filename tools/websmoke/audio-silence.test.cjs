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

async function evaluate(rms, isDemo, depth = 0, overrides = {}, diagnostics = {}) {
    const failures = [];
    const logs = [];
    const expressions = [];
    let install;
    const stats = {running:true, queued:2048, shared:false, rms, out:rms, muted:false, quiet:false, backend:'worker', consumed:1000, blocks:10,
        underruns:0, pumpUnderruns:0, targetBlocks:depth || 6, blockFrames:512, rate:48000, mixMaxUs:50, ...overrides};
    const page = {on(){}, goto:async()=>{}, close:async()=>{}, reload:async()=>{},
        addInitScript:async fn=>{install=fn; if (diagnostics.initError) throw new Error('observer unavailable');},
        mouse:{click:async()=>{}}, isHidden:async()=>false,
        click:async selector=>{if (selector === '#mute') {stats.muted=true;stats.out=0;}}};
    const scope = {
        fs:{existsSync:()=>true}, path, site:'/diagnostic-only', seconds:10, demoSeconds:10,
        sleep:async()=>{}, fail:s=>failures.push(s), console:{log:s=>logs.push(s)},
        serve:async()=>({address:()=>({port:1}),close(){}}),
        scriptWithoutGesture:async()=>({until:async()=>{}, js:async expression=>{
            expressions.push(expression);
            if (expression==='globalThis.crossOriginIsolated === true') return false;
            if (expression.includes('const c = new AudioContext()')) return false;
            if (expression.startsWith('globalThis.aegoAudioBlocks =')) return undefined;
            if (expression==='globalThis.aegoAudio.start()') return '';
            if (expression==='globalThis.aegoAudio.stats()' || expression==='globalThis.aegoAudioStats()') return JSON.stringify(stats);
            if (expression==='globalThis.aegoAudio.stop()' || expression.includes('globalThis.aegoAudioMute(false)')) return undefined;
            if (expression==="document.documentElement.dataset.sound === 'off'") return false;
            if (expression.includes('navigator.userActivation && navigator.userActivation.hasBeenActive')) return false;
            if (expression==='globalThis.aegoGame?.mixStats?.() ?? null') return {underruns:stats.pumpUnderruns, blocks:stats.blocks};
            if (expression.includes('while (performance.now() - start < 500)')) {
                if (!diagnostics.stallNoProgress) { stats.consumed+=24000;stats.blocks+=48; }
                if (diagnostics.stallUnderruns !== undefined) stats.underruns=diagnostics.stallUnderruns;
                if (diagnostics.stallPumpUnderruns !== undefined) stats.pumpUnderruns=diagnostics.stallPumpUnderruns;
                if (diagnostics.stallSilent) stats.quiet=true;
                return diagnostics.stallDuration ?? 500;
            }
            if (expression==='JSON.stringify({now:performance.now(), timing:globalThis.__aegoAudioSmokeTiming ?? null})') {
                if (diagnostics.readError) throw new Error('timing unavailable');
                return JSON.stringify({now:123, timing:{available:true, count:1, maxMS:160, tasks:[{startMS:50,durationMS:160}]}});
            }
            throw new Error('unexpected test expression: '+expression);
        }}),
    };
    vm.createContext(scope);
    vm.runInContext(code,scope,{filename:file});
    const browser={newPage:async()=>page};
    if (isDemo) await scope.demo(browser,'demo',{},false);
    else await scope.run(browser,'messages',{},false,depth);
    return {failures, logs, install, expressions};
}

test('tone silence fails even when both underrun counters are zero',async()=>{
    assert((await evaluate(0,false)).failures.some(x=>x.includes('rms 0')));
    assert((await evaluate(0,false,5)).failures.some(x=>x.includes('rms 0')));
});
test('game silence fails even when both underrun counters are zero',async()=>{
    assert((await evaluate(0,true)).failures.some(x=>x.includes('music is silent')));
});
test('a quiet or unreported worklet fails the actual demo gate even with audible history',async()=>{
    for(const quiet of [true,undefined]) {
        const result=await evaluate(0.09,true,0,{quiet});
        assert(result.failures.some(x=>x.includes('worklet is intentionally silent')));
    }
});
test('a local backend fails the actual demo gate',async()=>{
    const result=await evaluate(0.09,true,0,{backend:'local'});
    assert(result.failures.some(x=>x.includes('not using the audio Worker')));
});
test('the actual demo gate rejects stalled, quiet, or unreported worker output',async()=>{
    const cases=[
        [{stallNoProgress:true},'did not advance'],
        [{stallUnderruns:1},'worklet underruns after'],
        [{stallPumpUnderruns:1},'predicted pump underruns after'],
        [{stallSilent:true},'worklet is intentionally silent'],
        [{stallDuration:499},'stall did not run'],
    ];
    for(const [fault,expected] of cases) {
        const result=await evaluate(0.09,true,0,{},fault);
        assert(result.failures.some(x=>x.includes(expected)),`${expected}: ${result.failures}`);
    }
});
test('audible samples and zero underruns do not fail the fake checks',async()=>{
    assert.deepEqual((await evaluate(0.354,false)).failures,[]);
    const result=await evaluate(0.09,true);
    assert.deepEqual(result.failures,[]);
    assert(!result.logs.some(x=>x.includes('underrun timing')));
});
test('game underruns still fail and report the browser timing diagnostic',async()=>{
    const {failures,logs}=await evaluate(0.09,true,0,{underruns:1,pumpUnderruns:1});
    assert(failures.some(x=>x.includes('1 underruns in the game')));
    assert(failures.some(x=>x.includes('1 predicted pump underruns')));
    assert(logs.some(x=>x.includes('underrun timing') && x.includes('160')));
});
test('timing marks preserve the unmute and pre-click activation checks',async()=>{
    const {expressions}=await evaluate(0.09,true);
    const unmute=expressions.find(x=>x.includes('globalThis.aegoAudioMute(false)'));
    const activation=expressions.find(x=>x.includes('navigator.userActivation && navigator.userActivation.hasBeenActive'));
    let calls=0;
    const env={__aegoAudioSmokeTiming:{},performance:{now:()=>123},
        aegoAudioMute:on=>{assert.equal(on,false);calls++;return 'original result';},
        navigator:{userActivation:{hasBeenActive:false}}};
    assert.equal(vm.runInNewContext(unmute,env),'original result');
    assert.equal(calls,1);
    assert.equal(env.__aegoAudioSmokeTiming.unmuteMS,123);
    env.performance.now=()=>456;
    assert.equal(vm.runInNewContext(activation,env),false);
    assert.equal(env.__aegoAudioSmokeTiming.beforeClickSampleMS,456);
    delete env.__aegoAudioSmokeTiming;
    assert.equal(vm.runInNewContext(unmute,env),'original result');
    assert.equal(vm.runInNewContext(activation,env),false);
    assert.equal(calls,2);
});
test('unavailable timing never hides an underrun or fails a healthy game',async()=>{
    const failed=await evaluate(0.09,true,0,{underruns:1},{readError:true});
    assert(failed.failures.some(x=>x.includes('1 underruns in the game')));
    assert(failed.logs.some(x=>x.includes('underrun timing') && x.includes('unavailable')));
    assert.deepEqual((await evaluate(0.09,true,0,{}, {initError:true})).failures,[]);
});
test('long-task observer is bounded and records only the trusted first click',async()=>{
    const {install}=await evaluate(0.09,true);
    assert.equal(typeof install,'function');
    let observe, click, callback, pulse;
    class Observer {
        static supportedEntryTypes=['longtask'];
        constructor(fn) { callback=fn; }
        observe(options) { observe=options.type; }
    }
    const env={PerformanceObserver:Observer, performance:{now:()=>123},
        setInterval:(fn,ms)=>{assert.equal(ms,50);pulse=fn;},
        window:{addEventListener:(name,fn)=>{assert.equal(name,'pointerdown');click=fn;}}};
    vm.runInNewContext(`(${install.toString()})()`,env);
    const timing=env.__aegoAudioSmokeTiming;
    assert.equal(observe,'longtask');
    assert.equal(timing.available,true);
    click({isTrusted:false});
    assert.equal(timing.firstClickMS,null);
    click({isTrusted:true});
    env.performance.now=()=>456;
    click({isTrusted:true});
    assert.equal(timing.firstClickMS,123);
    pulse();
    assert.equal(timing.heartbeatAvailable,true);
    assert.equal(timing.gapCount,1);
    assert.equal(timing.maxGapMS,333);
    for (let i=0; i<100; i++) {
        env.performance.now=()=>656+i*200;
        pulse();
    }
    assert.equal(timing.gapCount,101);
    assert.equal(timing.maxGapMS,333);
    assert.equal(timing.gaps.length,32);
    assert.equal(timing.gaps[0].gapMS,333);
    assert.equal(timing.gaps[31].gapMS,200);
    callback({getEntries:()=>Array.from({length:100},(_,i)=>({startTime:i*100,duration:50+i}))});
    assert.equal(timing.count,100);
    assert.equal(timing.maxMS,149);
    assert.equal(timing.tasks.length,32);
    assert.equal(timing.tasks[0].startMS,0);
    assert.equal(timing.tasks[15].startMS,1500);
    assert.equal(timing.tasks[16].startMS,8400);
    assert.equal(timing.tasks[31].startMS,9900);
});
test('an unsupported or throwing long-task observer is diagnostic only',async()=>{
    const {install}=await evaluate(0.09,true);
    for (const Observer of [undefined, class {
        static supportedEntryTypes=['longtask'];
        observe() {throw new Error('unsupported');}
    }]) {
        const env={PerformanceObserver:Observer,performance:{now:()=>0},setInterval(){},window:{addEventListener(){}}};
        vm.runInNewContext(`(${install.toString()})()`,env);
        assert.equal(env.__aegoAudioSmokeTiming.available,false);
    }
});
