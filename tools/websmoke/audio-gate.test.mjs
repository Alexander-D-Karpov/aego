// Run the real browser-audio gate with a deterministic fake page. This checks
// policy only; it does not replace the actual browser/audio smoke tests.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import {test} from 'node:test';
import vm from 'node:vm';
import {fileURLToPath} from 'node:url';

const file=fileURLToPath(new URL('./audio.mjs',import.meta.url));
const source=fs.readFileSync(file,'utf8');
const from=source.indexOf('async function run(browser, label, headers, wantShared, depth = 0) {');
const to=source.indexOf('\nasync function demo(',from);
if (from<0 || to<0 || source.indexOf('async function run(browser, label, headers, wantShared, depth = 0) {',from+1)>=0)
    throw new Error('browser audio gate anchor changed');
const code=source.slice(from,to);

async function evaluate({depth=4, underruns=0, pumpUnderruns=0}={}) {
    const failures=[];
    const stats={running:true,queued:2048,shared:false,rms:0.354,targetBlocks:4,
                 blockFrames:512,rate:48000,underruns,pumpUnderruns,mixMaxUs:50};
    const page={on(){},goto:async()=>{},close:async()=>{},mouse:{click:async()=>{}}};
    const scope={
        site:'/diagnostic-only',
        seconds:10,
        fail:s=>failures.push(s),
        sleep:async()=>{},
        serve:async()=>({address:()=>({port:1}),close(){}}),
        scriptWithoutGesture:async()=>({
            until:async()=>{},
            js:async expression=>{
                if (expression==='globalThis.crossOriginIsolated === true') return false;
                if (expression.includes('const c = new AudioContext()')) return false;
                if (expression.startsWith('globalThis.aegoAudioBlocks =')) return undefined;
                if (expression==='globalThis.aegoAudio.start()') return '';
                if (expression==='globalThis.aegoAudio.stats()') return JSON.stringify(stats);
                if (expression==='globalThis.aegoAudio.stop()') return undefined;
                throw new Error('unexpected test expression: '+expression);
            },
        }),
        console:{log(){}},
    };
    vm.createContext(scope);
    vm.runInContext(code,scope,{filename:file});
    await scope.run({newPage:async()=>page},'messages at 4 blocks',{},false,depth);
    return failures;
}

test('optional queue-depth runs fail on a genuine worklet underrun',async()=>{
    const errors=await evaluate({depth:4,underruns:1});
    assert(errors.some(x=>x.includes('1 underruns')),'actual underrun did not fail the check');
});
test('optional queue-depth runs fail on a pump empty-queue estimate',async()=>{
    const errors=await evaluate({depth:4,pumpUnderruns:1});
    assert(errors.some(x=>x.includes('1 predicted pump underruns')),'pump underrun did not fail the check');
});
test('default target remains strict',async()=>{
    assert((await evaluate({depth:0,underruns:1})).some(x=>x.includes('1 underruns')));
    assert((await evaluate({depth:0,pumpUnderruns:1})).some(x=>x.includes('1 predicted pump underruns')));
});
test('valid zero-underrun samples pass',async()=>{
    assert.deepEqual(await evaluate({depth:4}),[]);
    assert.deepEqual(await evaluate({depth:0}),[]);
});
test('invalid worklet and pump counts fail closed',async()=>{
    assert((await evaluate({underruns:-1})).some(x=>x.includes('invalid worklet underrun count')));
    assert((await evaluate({pumpUnderruns:-1})).some(x=>x.includes('invalid pump underrun count')));
});

test('audible demo rejects a quiet or unreported worklet',()=>{
    for(const quiet of [true,undefined]) {
        const failures=[];
        const scope={fail:message=>failures.push(message)};
        vm.createContext(scope);
        vm.runInContext(code,scope,{filename:file});
        scope.checkAudibleWorklet({quiet,backend:'worker'},'demo');
        assert(failures.some(message=>message.includes('intentionally silent')));
    }
    const failures=[];
    const scope={fail:message=>failures.push(message)};
    vm.createContext(scope);
    vm.runInContext(code,scope,{filename:file});
    scope.checkAudibleWorklet({quiet:false,backend:'worker'},'demo');
    assert.deepEqual(failures,[]);
});

test('audible demo requires the Worker rather than the old local backend',()=>{
    for(const backend of ['local','worker-starting',undefined]) {
        const failures=[];
        const scope={fail:message=>failures.push(message)};
        vm.createContext(scope);
        vm.runInContext(code,scope,{filename:file});
        scope.checkAudibleWorklet({quiet:false,backend},'demo');
        assert(failures.some(message=>message.includes('not using the audio Worker')));
    }
});
