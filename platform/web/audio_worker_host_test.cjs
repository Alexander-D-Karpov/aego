'use strict';

// Tests the page-side ownership and ordering boundary with fake ports and a
// fake Worker. The real Worker, WASI reactor and AudioWorklet are tested apart.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const {test} = require('node:test');
const vm = require('node:vm');

const code = fs.readFileSync(path.join(__dirname, 'audio_worker_host.js'), 'utf8');
const scope = {Float32Array, Number, JSON, Error};
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
    h.receive({type:'batch', batch:JSON.stringify({serial})});
    assert.equal(h.events.at(-1).type, 'batch');
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
    const id = h.host.rebind({node, block:512, target:3072, rate:44100});
    assert.equal(h.sent.at(-1).message.op, 'rebind');
    assert.equal(h.sent.at(-1).message.port, h.channels[1].port1);
    assert.equal(node.port.sent.length, 0);
    h.receive({type:'reply', id, result:''});
    assert.equal(node.port.sent[0].message.producerPort, h.channels[1].port2);
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
