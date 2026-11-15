'use strict';

// Tests the page-side ownership and ordering boundary with fake ports and a
// fake Worker. The real Worker, WASI reactor and AudioWorklet are tested apart.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const {test} = require('node:test');
const vm = require('node:vm');

const code = fs.readFileSync(path.join(__dirname, 'audio_worker_host.js'), 'utf8');
const scope = {Float32Array, Uint8Array, Number, JSON, Error, URL, AbortController, setTimeout, clearTimeout};
vm.createContext(scope);
vm.runInContext(code, scope, {filename:'audio_worker_host.js'});

function harness() {
    const sent = [], events = [], channels = [];
    class Channel {
        constructor() {
            const port = () => ({closed:false, close(){this.closed = true;}});
            this.port1 = port(); this.port2 = port(); channels.push(this);
        }
    }
    const worker = {
        onmessage:null, onerror:null, onmessageerror:null, stopped:0,
        postMessage(message, transfer) {sent.push({message, transfer});},
        terminate() {this.stopped++;},
    };
    const node = {port:{sent:[], postMessage(message, transfer){this.sent.push({message, transfer});}}};
    const host = new scope.AegoAudioWorkerHost(worker, event => events.push(event), Channel);
    const start = () => host.start({module:{}, node, input:'{}', block:512, target:3072});
    const receive = message => worker.onmessage({data:message});
    return {host,worker,node,sent,events,channels,start,receive};
}

test('startup failure leaves the output unattached and terminates the Worker', () => {
    const h = harness();
    h.start();
    assert.equal(h.sent[0].message.type, 'init');
    assert.equal(h.sent[0].message.port, h.channels[0].port1);
    assert.equal(h.node.port.sent.length, 0);
    h.receive({type:'fatal', error:'ABI mismatch'});
    assert.equal(h.host.state, 'failed');
    assert.equal(h.events.length, 1);
    assert.equal(h.events[0].attached, false);
    assert.match(h.events[0].error, /ABI mismatch/);
    assert.equal(h.channels[0].port2.closed, true);
    assert.equal(h.worker.stopped, 1);
});

test('ready attaches a dedicated producer port before exposing readiness', () => {
    const h = harness();
    h.start();
    h.receive({type:'ready'});
    assert.equal(h.host.state, 'ready');
    assert.equal(h.node.port.sent.length, 1);
    assert.equal(h.node.port.sent[0].message.producerPort, h.channels[0].port2);
    assert.equal(h.events[0].type, 'ready');
    h.receive({type:'ready'});
    assert.equal(h.host.state, 'failed');
    assert.equal(h.events.at(-1).attached, true);
});

test('an uncertain output attachment never claims safe local fallback', () => {
    const h = harness();
    h.node.port.postMessage = () => { throw Error('injected attachment failure'); };
    h.start();
    h.receive({type:'ready'});
    assert.equal(h.host.state, 'failed');
    assert.equal(h.worker.stopped, 1);
    assert.equal(h.channels[0].port2.closed, true);
    assert.equal(h.events.at(-1).attached, true);
});

test('channel allocation failure stops startup or rebind without guessing state', () => {
    class BrokenChannel {constructor(){throw Error('injected channel failure');}}
    const startup = harness();
    startup.host.Channel = BrokenChannel;
    startup.start();
    assert.equal(startup.host.state, 'failed');
    assert.equal(startup.events.at(-1).attached, false);
    assert.equal(startup.worker.stopped, 1);

    const rebind = harness(); rebind.start(); rebind.receive({type:'ready'});
    rebind.host.Channel = BrokenChannel;
    const node = {port:{postMessage(){}}};
    assert.equal(rebind.host.rebind({node, block:512, target:3072, rate:44100}), 1);
    assert.equal(rebind.host.state, 'failed');
    assert.equal(rebind.events.at(-1).attached, true);
    assert.equal(rebind.worker.stopped, 1);
});

test('RPCs are single-flight, preserve PCM and reject stale replies', () => {
    const h = harness(); h.start(); h.receive({type:'ready'});
    const pcm = new Float32Array([.1,.2,.3]);
    const id = h.host.send('memory-chunk', '{}', pcm, 4);
    const sent = h.sent.at(-1);
    assert.equal(id, 1);
    assert.equal(sent.message.offset, 4);
    assert.notEqual(sent.message.pcm, pcm);
    assert.deepEqual(Array.from(sent.message.pcm), Array.from(pcm));
    pcm[0] = 7;
    assert.notEqual(sent.message.pcm[0], pcm[0]);
    assert.equal(sent.transfer[0], sent.message.pcm.buffer);
    assert.throws(() => h.host.send(8, '{}'), /in flight/);
    h.receive({type:'reply', id, result:''});
    assert.equal(h.events.at(-1).type, 'reply');
    assert.equal(h.host.send(8, '{}'), 2);
    h.receive({type:'reply', id:1, result:''});
    assert.equal(h.host.state, 'failed');
    assert.equal(h.events.at(-1).attached, true);
});

test('batch acknowledgements retain exact uint64 serials and apply backpressure', () => {
    const h = harness(); h.start(); h.receive({type:'ready'});
    const serial = '9007199254740993';
    const stats={blocks:7,underruns:1,maxMixMS:.1,totalMixMS:.3};
    h.receive({type:'batch', batch:JSON.stringify({serial}),stats});
    assert.equal(h.events.at(-1).type, 'batch');
    assert.equal(h.events.at(-1).stats,stats);
    h.host.ackBatch(serial);
    assert.equal(h.sent.at(-1).message.serial, serial);
    h.receive({type:'batch', batch:JSON.stringify({serial:'9007199254740994'})});
    h.receive({type:'batch', batch:JSON.stringify({serial:'9007199254740995'})});
    assert.equal(h.host.state, 'failed');
    assert.match(h.events.at(-1).error, /overlapping/);

    const wrong = harness(); wrong.start(); wrong.receive({type:'ready'});
    wrong.receive({type:'batch', batch:JSON.stringify({serial})});
    assert.throws(() => wrong.host.ackBatch('9007199254740992'), /acknowledgement/);
    assert.equal(wrong.sent.filter(x => x.message.type === 'batch-ack').length, 0);
    assert.equal(wrong.host.state, 'failed');
});

test('rebind attaches the new producer only after the ordered reply', () => {
    const h = harness(); h.start(); h.receive({type:'ready'});
    const node = {port:{sent:[],postMessage(message, transfer){this.sent.push({message, transfer});}}};
    const id = h.host.rebind({node, block:512, target:3072, rate:44100, quiet:true});
    assert.equal(h.sent.at(-1).message.op, 'rebind');
    assert.equal(h.sent.at(-1).message.quiet, true);
    assert.equal(h.sent.at(-1).message.port, h.channels[1].port1);
    assert.equal(node.port.sent.length, 0);
    h.receive({type:'reply', id, result:''});
    assert.equal(node.port.sent[0].message.producerPort, h.channels[1].port2);
    assert.equal(node.port.sent[0].message.quiet, true);
    assert.equal(h.host.node, node);
    assert.equal(h.events.at(-1).type, 'reply');
    assert.throws(() => h.host.rebind({node, block:512, target:3072, rate:44100}), /rebind/);
});

test('quiesce is serialized with RPCs and reports only the Worker reply', () => {
    const h=harness();h.start();h.receive({type:'ready'});
    const id=h.host.send(8,'{}');
    assert.throws(()=>h.host.quiesce(),/in flight/);
    h.receive({type:'reply',id,result:''});
    const next=h.host.quiesce();
    assert.equal(next,2);
    assert.equal(h.sent.at(-1).message.op,'quiesce');
    assert.equal(h.events.filter(event=>event.type==='reply').length,1);
    assert.throws(()=>h.host.send(11,'{}'),/in flight/);
    h.receive({type:'reply',id:next,result:''});
    assert.equal(h.events.filter(event=>event.type==='reply').length,2);
    assert.equal(h.host.send(11,'{"frames":1}'),3);
});

test('bounded quiet skips use the same single-flight ordered RPC', () => {
    const h=harness();h.start();h.receive({type:'ready'});
    for (const bad of [-1, 1.5, (1 << 20) + 1, Infinity, NaN, '3']) {
        assert.throws(()=>h.host.skip(bad),/invalid audio worker skip/);
    }
    assert.equal(h.sent.length,1,'invalid skips must not cross the transport');
    const zero=h.host.skip(0);
    assert.equal(h.sent.at(-1).message.op,11);
    assert.deepEqual(JSON.parse(h.sent.at(-1).message.input),{});
    assert.throws(()=>h.host.skip(1),/in flight/);
    h.receive({type:'reply',id:zero,result:''});
    const max=h.host.skip(1<<20);
    assert.equal(max,2);
    assert.equal(JSON.parse(h.sent.at(-1).message.input).frames,1<<20);
    h.receive({type:'reply',id:max,result:''});
    assert.equal(h.host.state,'ready');
});

test('bad transport messages and consumer failures stop rather than replay', () => {
    for (const bad of [{type:'reply',id:1,result:''},{type:'batch',batch:'{"serial":3}'},null]) {
        const h = harness(); h.start(); h.receive({type:'ready'}); h.receive(bad);
        assert.equal(h.host.state, 'failed');
        assert.equal(h.worker.stopped, 1);
        assert.equal(h.events.at(-1).type, 'fatal');
    }
    const h = harness(); h.start(); h.receive({type:'ready'});
    h.host.notify = () => {throw Error('consumer failure');};
    h.receive({type:'batch',batch:'{"serial":"1"}'});
    assert.equal(h.host.state, 'failed');
    assert.equal(h.worker.stopped, 1);
});

test('reactor preparation is same-origin and leaves fallback on failure', async () => {
    const module={compiled:true},fetched=[];
    scope.location={href:'https://example.test/demo/index.html',origin:'https://example.test'};
    scope.Worker=function(){};
    scope.WebAssembly={compile:async bytes=>{assert.equal(bytes.byteLength,3);return module;}};
    scope.fetch=async (url, options)=>{fetched.push({url,options});return {ok:true,headers:{get:()=>3},arrayBuffer:async()=>new ArrayBuffer(3)};};
    const ready=await scope.AegoPrepareAudioWorker('audio-mixer.wasm?v=123','audio-worker.js?v=123');
    assert.equal(ready.module,module);
    assert.equal(ready.url,'https://example.test/demo/audio-worker.js?v=123');
    assert.equal(fetched[0].url,'https://example.test/demo/audio-mixer.wasm?v=123');
    assert(fetched[0].options.signal instanceof AbortSignal);
    assert.equal(fetched[0].options.mode,'same-origin');
    assert.equal(fetched[0].options.redirect,'error');
    assert.equal(await scope.AegoPrepareAudioWorker('https://foreign.test/audio.wasm','audio-worker.js'),null);
    assert.equal(await scope.AegoPrepareAudioWorker('https://user@example.test/audio.wasm','audio-worker.js'),null);
    assert.equal(fetched.length,1,'cross-origin reactor was fetched');
    scope.fetch=async()=>({ok:false,status:404});
    assert.equal(await scope.AegoPrepareAudioWorker('audio-mixer.wasm','audio-worker.js'),null);
    let redirected;
    scope.fetch=async(_url,options)=>{redirected=options.signal;return {ok:true,redirected:true,url:'https://foreign.test/audio.wasm'};};
    assert.equal(await scope.AegoPrepareAudioWorker('audio-mixer.wasm','audio-worker.js'),null);
    assert.equal(redirected.aborted,true,'rejected response left its fetch active');
    scope.fetch=async()=>({ok:true,headers:{get:()=>String(33<<20)}});
    assert.equal(await scope.AegoPrepareAudioWorker('audio-mixer.wasm','audio-worker.js'),null);
    scope.fetch=async()=>({ok:true,headers:{get:()=>null},body:{getReader(){
        let done=false;return {async read(){if(done)return {done:true};done=true;return {done:false,value:new Uint8Array([1,2,3])};}};
    }}});
    assert.equal((await scope.AegoPrepareAudioWorker('audio-mixer.wasm','audio-worker.js')).module,module);
    scope.WebAssembly={compile:()=>new Promise(()=>{})};
    assert.equal(await scope.AegoPrepareAudioWorker('audio-mixer.wasm','audio-worker.js',5),null,'hung compilation did not time out');
    scope.Worker=undefined;
    assert.equal(await scope.AegoPrepareAudioWorker('audio-mixer.wasm','audio-worker.js'),null);
});
