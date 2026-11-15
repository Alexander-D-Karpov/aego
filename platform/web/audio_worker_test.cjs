'use strict';

// Exercise the Worker transport with a deterministic WASI ABI double. Real
// Go/WASM, browser AudioWorklet and device checks remain separate gates.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const {test} = require('node:test');
const vm = require('node:vm');
const {performance} = require('node:perf_hooks');
const {webcrypto} = require('node:crypto');

const code = fs.readFileSync(path.join(__dirname, 'audio_worker.js'), 'utf8');

function harness({shared = false, target = 3072, version = 4, active = 0} = {}) {
    const memory = new WebAssembly.Memory({initial:32});
    const enc = new TextEncoder(), dec = new TextDecoder();
    const messages = [], calls = [], output = [], errors = [];
    let inputLength = 0, resultLength = 0, serial = 0, brokenRender = false, brokenOp = 0;
    const write = (ptr, text) => {
        const raw = enc.encode(text);
        new Uint8Array(memory.buffer, ptr, raw.length).set(raw);
        return raw.length;
    };
    const fake = {
        memory,
        _initialize() {},
        aego_abi_version() {return version;},
        aego_input(n) { inputLength = n; return 4096; },
        aego_call(op) {
            if (op === brokenOp) return 0;
            const input = dec.decode(new Uint8Array(memory.buffer, 4096, inputLength));
            const value = JSON.parse(input);
            calls.push({op,value});
            let result = '';
            if (op === 10) result = JSON.stringify({serial:String(++serial),active});
            if (op === 5) result = JSON.stringify({frames:value.samples});
            resultLength = write(8192, result);
            return 1;
        },
        aego_memory() { return 65536; },
        aego_stream() { return 393216; },
        aego_render(n) {
            if (brokenRender) return 0;
            new Float32Array(memory.buffer, 786432, n * 2).fill(.25);
            return 786432;
        },
        aego_result() { return 8192; },
        aego_result_size() { return resultLength; },
        aego_error() { return 12288; },
        aego_error_size() { return write(12288, 'injected reactor failure'); },
    };
    const port = {onmessage:null, start(){}, close(){this.closed = true;},
        postMessage(value) { output.push(value); }};
    const scope = {
        TextEncoder, TextDecoder, Float32Array, Int32Array, Uint8Array, ArrayBuffer, SharedArrayBuffer,
        Atomics, Number, BigInt, JSON, Error, Math, performance, crypto:webcrypto,
        WebAssembly:{Instance:class {constructor() {this.exports = fake;}}},
        onmessage:null, postMessage:value => messages.push(value), close:()=>errors.push('closed'),
    };
    vm.createContext(scope);
    vm.runInContext(code, scope, {filename:'audio_worker.js'});
    const data = shared ? new SharedArrayBuffer(8192 * 2 * 4) : null;
    const index = shared ? new SharedArrayBuffer(16) : null;
    const send = message => scope.onmessage({data:message});
    send({type:'init', module:{}, port, data, index, block:512, target,
        input:JSON.stringify({rate:48000,frames:512,voices:4,budget:1<<20})});
    return {send,port,messages,calls,output,memory,index,data,errors,
        breakRender:()=>{brokenRender = true;},
        breakOp:op=>{brokenOp = op;},
        rpc(id,op,value,pcm,offset) {send({type:'rpc',id,op,input:JSON.stringify(value),pcm,offset});},
        ack(serial) {send({type:'batch-ack',serial});},
        start() {port.onmessage({data:{ready:true}});},
        consume(n) {if(shared)Atomics.store(new Int32Array(index),0,n);port.onmessage({data:{consumed:n}});},
    };
}

test('a mismatched reactor ABI fails before the output is attached', () => {
    const h=harness({version:3});
    assert.equal(h.messages.length,1);
    assert.equal(h.messages[0].type,'fatal');
    assert.match(h.messages[0].error,/ABI version mismatch/);
    assert.equal(h.port.onmessage,null);
    assert.equal(h.calls.length,0);
});

test('replacement commands preserve exact frame and previous voice fields', () => {
    const h = harness();
    const command = {sequence:'1', kind:16, index:1, gen:2, source:'7',
        frame:'9007199254741011', prevIndex:3, prevGen:4, paused:true};
    h.rpc(1, 8, {command});
    assert.deepEqual(h.calls.at(-1).value.command, command);
    assert.notEqual(h.messages.at(-1).type, 'fatal');
});

test('startup primes bounded PCM and refills without the page thread', () => {
    const h = harness();
    assert.equal(h.messages[0].type, 'ready');
    assert.equal(h.output.length, 0);
    h.start();
    assert.equal(h.output.length, 1);
    assert.equal(h.output[0].length, 3072 * 2);
    assert(h.output[0].every(value => value === .25));
    assert.equal(h.messages[1].type, 'batch');
    assert.equal(JSON.parse(h.messages[1].batch).serial, '1');
    h.ack('1');
    h.consume(1024);
    assert.equal(h.output.length, 2);
    assert.equal(h.output[1].length, 1024 * 2);
    assert.equal(JSON.parse(h.messages[2].batch).serial, '2');
    assert.deepEqual(h.calls.filter(call => call.op === 10).map(call => call.op), [10,10]);
});

test('a full shared ring waits for a complete block of space before mixing', () => {
    const h = harness({shared:true,target:8192});
    h.start();
    assert.equal(Atomics.load(new Int32Array(h.index),1),8192);
    h.consume(128);
    assert.equal(Atomics.load(new Int32Array(h.index),1),8192);
    assert.notEqual(h.messages.at(-1).type,'fatal');
    h.consume(512);
    assert.equal(Atomics.load(new Int32Array(h.index),1),8704);
    assert.notEqual(h.messages.at(-1).type,'fatal');
});

test('page stalls retain one outstanding batch while audio keeps refilling', () => {
    const h = harness();
    h.start();
    for (let consumed=512;consumed<=5120;consumed+=512) h.consume(consumed);
    assert.equal(h.messages.filter(value=>value.type==='batch').length,1);
    assert.equal(h.output.length,11);
    h.ack('1');
    const batches=h.messages.filter(value=>value.type==='batch');
    assert.equal(batches.length,2);
    assert.equal(JSON.parse(batches[1].batch).serial,'2');
    h.ack('1');
    assert.equal(h.messages.at(-1).type,'fatal');
});

test('worker mixer statistics retain bounded queue-empty diagnostics', () => {
    const h=harness({active:1});
    h.start();
    let batch=h.messages.findLast(value=>value.type==='batch');
    assert.equal(batch.stats.blocks,6);
    assert.equal(batch.stats.underruns,0);
    assert(batch.stats.maxMixMS>=0 && batch.stats.totalMixMS>=batch.stats.maxMixMS);
    h.ack('1');
    h.consume(3072);
    batch=h.messages.findLast(value=>value.type==='batch');
    assert.equal(batch.stats.blocks,12);
    assert.equal(batch.stats.underruns,1,'an empty active queue was not reported');
    h.ack('2');
    h.rpc(1,'quiesce',{});
    h.port.onmessage({data:{quiesced:1,consumed:3072}});
    h.rpc(2,11,{frames:1024});
    batch=h.messages.findLast(value=>value.type==='batch');
    assert.equal(batch.stats.underruns,1,'intentional quiet changed previous diagnostics');
    h.ack('3');
    h.rpc(3,'resume',{});
    h.port.onmessage({data:{resumed:3,consumed:3072}});
    batch=h.messages.findLast(value=>value.type==='batch');
    assert.equal(batch.stats.underruns,1,'fresh priming was counted as a shortage');
});

test('silent skip publishes state even when the worklet has not started', () => {
    const h=harness();
    h.rpc(1,'quiesce',{});
    assert.equal(h.output.at(-1).quiesce,1);
    h.port.onmessage({data:{quiesced:1,consumed:0}});
    h.rpc(2,11,{frames:512});
    const batch=h.messages.find(value=>value.type==='batch');
    assert.equal(JSON.parse(batch.batch).serial,'1');
    assert.equal(h.output.filter(value=>value instanceof Float32Array).length,0);
    h.rpc(3,11,{frames:512});
    assert.equal(h.messages.filter(value=>value.type==='batch').length,1);
    h.ack('1');
    assert.equal(JSON.parse(h.messages.at(-1).batch).serial,'2');
});

test('quiescence stops old refills and is required before silent skip', () => {
    for (const shared of [false,true]) {
        const h=harness({shared});
        h.start();
        h.ack('1');
        const oldReport=h.port.onmessage;
        const tail=shared ? Atomics.load(new Int32Array(h.index),1) : h.output.filter(value=>value instanceof Float32Array).length;
        h.rpc(1,11,{frames:128});
        assert.match(h.messages.at(-1).error,/quiesced/);
        assert.equal(h.calls.filter(call=>call.op===11).length,0);
        h.rpc(2,'quiesce',{});
        assert.equal(h.output.at(-1).quiesce,2);
        assert.notEqual(h.messages.at(-1).id,2,'quiesce replied before the worklet discarded old PCM');
        oldReport({data:{consumed:512}});
        if(shared)Atomics.store(new Int32Array(h.index),0,tail);
        h.port.onmessage({data:{quiesced:2,consumed:512}});
        assert.equal(h.messages.at(-1).id,2);
        h.rpc(3,11,{frames:128});
        assert.deepEqual(h.calls.filter(call=>call.op===11).map(call=>call.value.frames),[128]);
        assert.equal(shared ? Atomics.load(new Int32Array(h.index),1) : h.output.filter(value=>value instanceof Float32Array).length,tail);
        const batch=h.messages.findLast(value=>value.type==='batch');
        h.ack(JSON.parse(batch.batch).serial);
        h.rpc(4,'resume',{});
        assert.equal(h.output.at(-1).resume,4);
        assert.notEqual(h.messages.at(-1).id,4,'resume replied before the worklet accepted fresh PCM');
        assert.equal(shared ? Atomics.load(new Int32Array(h.index),1) : h.output.filter(value=>value instanceof Float32Array).length,
            shared ? tail+3072 : tail+1);
        h.port.onmessage({data:{resumed:4,consumed:512}});
        assert.equal(h.messages.at(-1).id,4);
    }
});

test('invalid or overlapping output transitions fail without resuming audio', () => {
    const wrong=harness();
    wrong.start();
    wrong.rpc(1,'quiesce',{});
    wrong.port.onmessage({data:{quiesced:2,consumed:0}});
    assert.equal(wrong.messages.at(-1).type,'fatal');
    assert.match(wrong.messages.at(-1).error,/quiescence acknowledgement/);
    assert.equal(wrong.messages.filter(value=>value.type==='reply'&&value.id===1).length,0);

    const overlap=harness();
    overlap.start();
    overlap.rpc(1,'quiesce',{});
    overlap.rpc(2,11,{frames:512});
    assert.equal(overlap.messages.at(-1).type,'fatal');
    assert.match(overlap.messages.at(-1).error,/transition is in flight/);
    assert.equal(overlap.calls.filter(call=>call.op===11).length,0);

    const resume=harness();
    resume.start();
    resume.rpc(1,'resume',{});
    assert.match(resume.messages.at(-1).error,/not quiesced/);
});

test('shared transport publishes a bounded ring and does not overrun it', () => {
    const h = harness({shared:true, target:1536});
    h.start();
    assert.equal(Atomics.load(new Int32Array(h.index), 1), 1536);
    assert.equal(h.output.length, 0);
    assert.equal(new Float32Array(h.data)[0], .25);
    h.consume(1024);
    assert.equal(Atomics.load(new Int32Array(h.index), 1), 2560);
    assert.equal(new Float32Array(h.data)[2 * 2048], .25);
});

test('refill acknowledgements follow publication, even for an already full queue', () => {
    const h = harness();
    h.start();
    h.port.onmessage({data:{consumed:0,refill:11}});
    assert.equal(h.output[1].refilled,11);
    h.port.onmessage({data:{consumed:1024,refill:12}});
    assert.equal(h.output[2].length,1024*2);
    assert.equal(h.output[3].refilled,12);
    h.port.onmessage({data:{consumed:1024,refill:-1}});
    assert.equal(h.messages.at(-1).type,'fatal');
});

test('memory transfer must be complete, contiguous and match its exact source ID', () => {
    const h = harness();
    const src = '9007199254740993';
    h.rpc(1,2,{source:src,rate:48000,channels:1,samples:4});
    h.rpc(2,'memory-chunk',{},new Float32Array([1,2]),0);
    h.rpc(3,3,{source:src});
    assert.match(h.messages.at(-1).error, /incomplete/);
    h.rpc(4,'memory-chunk',{},new Float32Array([3,4]),1);
    assert.match(h.messages.at(-1).error, /invalid staged PCM/);
    h.rpc(5,'memory-chunk',{},new Float32Array([3,4]),2);
    h.rpc(6,3,{source:'9007199254740992'});
    assert.match(h.messages.at(-1).error, /incomplete/);
    h.rpc(7,3,{source:src});
    assert.equal(h.messages.at(-1).error, undefined);
    assert.deepEqual(Array.from(new Float32Array(h.memory.buffer,65536,4)), [1,2,3,4]);
    assert.deepEqual(h.calls.filter(call => call.op === 3).map(call => call.value.source), [src]);
});

test('stream writes transfer exactly the offered PCM and return the service acknowledgement', () => {
    const h = harness(), pcm = new Float32Array([.1,.2,.3,.4]);
    h.rpc(1,5,{source:'2',samples:4},pcm);
    assert.deepEqual(Array.from(new Float32Array(h.memory.buffer,393216,4)), Array.from(pcm));
    assert.equal(JSON.parse(h.messages.at(-1).result).frames, 4);
    h.rpc(2,5,{source:'2',samples:5},pcm);
    assert.match(h.messages.at(-1).error, /invalid streaming PCM/);
    assert.equal(h.calls.filter(call => call.op === 5).length, 1);
});

test('device rebind preserves the mixer and ignores old worklet reports', () => {
    for (const wasShared of [false, true]) {
        const h = harness({shared:wasShared});
        h.start();
        h.ack('1');
        const oldReport = h.port.onmessage;
        const nextOutput = [];
        const nextPort = {onmessage:null, start(){}, close(){this.closed=true;},
            postMessage:value=>nextOutput.push(value)};
        const data = wasShared ? null : new SharedArrayBuffer(8192 * 2 * 4);
        const index = wasShared ? null : new SharedArrayBuffer(16);
        // Reject an invalid replacement before detaching the old device.
        h.send({type:'rpc', id:1, op:'rebind', port:nextPort, block:512, target:513, rate:44100, data, index});
        assert.match(h.messages.at(-1).error, /invalid audio worker output/);
        assert.equal(h.port.closed, undefined);
        assert.equal(h.calls.filter(call=>call.op===13).length, 0);
        h.consume(512);
        const priorSerial = JSON.parse(h.messages.findLast(value=>value.type==='batch').batch).serial;
        h.ack(priorSerial);
        h.send({type:'rpc', id:2, op:'rebind', port:nextPort, block:512, target:3072, rate:44100, data, index});
        assert.equal(h.messages.at(-1).type, 'reply');
        assert.equal(h.messages.at(-1).error, undefined);
        assert.equal(h.port.closed, true);
        assert.equal(h.port.onmessage, null);
        assert.deepEqual(h.calls.filter(call=>call.op===13).map(call=>call.value.rate), [44100]);
        nextPort.onmessage({data:{ready:true}});
        const afterSerial = JSON.parse(h.messages.findLast(value=>value.type==='batch').batch).serial;
        assert.equal(Number(afterSerial), Number(priorSerial)+1);
        if (data) {
            assert.equal(Atomics.load(new Int32Array(index),1), 3072);
        } else {
            assert.equal(nextOutput.length, 1);
            assert.equal(nextOutput[0].length, 3072*2);
        }
        const count = h.messages.length;
        oldReport({data:{consumed:999999999}});
        assert.equal(h.messages.length, count, 'stale reports affected the replacement output');
    }
});

test('quiet rebind accepts ordered skips before replacement starts mixing', () => {
    for(const shared of [false,true]) {
        const h=harness();
        h.start();h.ack('1');
        const nextOutput=[];
        const nextPort={onmessage:null,start(){},close(){this.closed=true;},postMessage:value=>nextOutput.push(value)};
        const data=shared ? new SharedArrayBuffer(8192*2*4) : null;
        const index=shared ? new SharedArrayBuffer(16) : null;
        h.send({type:'rpc',id:1,op:'rebind',port:nextPort,block:512,target:3072,rate:44100,data,index,quiet:true});
        assert.equal(h.messages.at(-1).id,1);
        h.rpc(2,11,{frames:480});
        const skipBatch=h.messages.findLast(value=>value.type==='batch');
        assert.equal(JSON.parse(skipBatch.batch).serial,'2');
        h.ack('2');
        nextPort.onmessage({data:{ready:true,quiet:true}});
        assert.equal(nextOutput.length,0,'quiet replacement was mixed before resume');
        if(shared)assert.equal(Atomics.load(new Int32Array(index),1),0);
        h.rpc(3,'resume',{});
        assert.equal(nextOutput.at(-1).resume,3);
        if(shared)assert.equal(Atomics.load(new Int32Array(index),1),3072);
        else assert.equal(nextOutput[0].length,3072*2);
        nextPort.onmessage({data:{resumed:3,consumed:0}});
        assert.equal(h.messages.at(-1).id,3);
    }
});

test('device rebind rejects reused ports and nonempty shared rings', () => {
    const h = harness();
    h.start();
    h.send({type:'rpc',id:1,op:'rebind',port:h.port,block:512,target:3072,rate:48000});
    assert.match(h.messages.at(-1).error, /invalid audio worker rebind/);
    const port = {postMessage(){},close(){},start(){}};
    const data = new SharedArrayBuffer(8192*2*4), index = new SharedArrayBuffer(16);
    Atomics.store(new Int32Array(index),1,512);
    h.send({type:'rpc',id:2,op:'rebind',port,data,index,block:512,target:3072,rate:48000});
    assert.match(h.messages.at(-1).error, /invalid shared audio ring/);
    assert.equal(h.port.closed, undefined);
    h.consume(512);
    assert.notEqual(h.messages.at(-1).type, 'fatal');
});

test('malformed ordering and render failures stop the transport', () => {
    const h = harness();
    h.rpc(2,8,{command:{sequence:'1'}});
    assert.equal(h.messages.at(-1).type, 'fatal');
    h.start();
    assert.equal(h.output.length, 0);
    const r = harness();
    r.start();
    r.breakRender();
    r.consume(1024);
    assert.equal(r.messages.at(-1).type, 'fatal');
    assert.match(r.messages.at(-1).error, /injected reactor failure/);
    assert.equal(r.output.length, 1);
});

test('reactor operation errors fail closed instead of continuing an unknown state', () => {
    const h=harness();
    h.breakOp(8);
    h.rpc(1,8,{command:{sequence:'1'}});
    assert.equal(h.messages.at(-2).type,'reply');
    assert.match(h.messages.at(-2).error,/injected reactor failure/);
    assert.equal(h.messages.at(-1).type,'fatal');
    h.rpc(2,8,{command:{sequence:'2'}});
    assert.equal(h.messages.filter(message=>message.type==='reply').length,1);
});
