import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";

const source = fs.readFileSync(new URL("../worklet.js", import.meta.url), "utf8");
for (const shared of [false, true]) {
    const messages = [];
    let Worklet;
    const env = {
        sampleRate: 48000, currentTime: 0, Float32Array,
        AudioWorkletProcessor: class {
            constructor() { this.port = { postMessage: data => messages.push(data) }; }
        },
        registerProcessor(name, Class) { assert.equal(name, "aego-out"); Worklet = Class; },
    };
    vm.runInNewContext(source, env);
    const options = { channels: 2, report: 2048, refillAt: 960 };
    if (shared) {
        options.data = new SharedArrayBuffer(8192 * 2 * 4);
        options.index = new SharedArrayBuffer(16);
    }
    const worklet = new Worklet({ processorOptions: options });
    const samples = new Float32Array(512 * 2).fill(.25);
    let sent = 0;
    function post(frames) {
        for (let n = 0; n < frames; n += 512) {
            if (shared) {
                const data = new Float32Array(options.data);
                data.set(samples, 2 * (sent % 8192));
                Atomics.add(new Int32Array(options.index), 1, 512);
            } else { worklet.port.onmessage({ data: samples }); }
            sent += 512;
        }
    }
    const channels = [[new Float32Array(128), new Float32Array(128)]];
    function read() {
        worklet.process([], channels);
        env.currentTime += 128 / 48000;
        return channels[0][0][0];
    }
    // Typical 60 Hz producer: requests never add a report to the regular cadence.
    post(2048);
    for (let quantum = 0; quantum < 192; quantum++) {
        assert.equal(read(), .25);
        if ((quantum + 1) % 6 === 0) {
            const queued = sent - worklet.consumed;
            post(Math.ceil((2048 - queued) / 512) * 512);
        }
    }
    assert.equal(messages.length, 12);
    assert.equal(worklet.underruns, 0);
    const before = messages.length;
    while (sent - worklet.consumed > 960) read();
    assert.equal(messages.length, before + 1);
    assert.ok(sent - messages.at(-1).consumed > 0, "request before physical silence");
    // A missing producer still creates and reports a genuine worklet underrun.
    while (sent > worklet.consumed) read();
    const drained = worklet.consumed;
    assert.equal(read(), 0);
    assert.equal(worklet.underruns, 1);
    assert.equal(worklet.consumed, drained);
    for (let n = 0; n < 16; n++) read();
    assert.equal(messages.at(-1).consumed, sent);
    assert.ok(messages.at(-1).underruns > 0);
    // No request storm at empty queue. A real refill rearms the next crossing.
    assert.ok(messages.length <= before + 3);
    post(2048);
    const previous = messages.length;
    while (sent - worklet.consumed > 960) read();
    assert.ok(messages.length > previous);
}

// The direct producer port receives the same real consumption reports as the
// main page. A stalled or disconnected producer still causes real underruns.
for (const shared of [false, true]) {
    let Worklet;
    const mainReports = [], workerReports = [];
    const env = {
        sampleRate: 48000, currentTime: 0, Float32Array,
        AudioWorkletProcessor: class { constructor() { this.port = {postMessage:data=>mainReports.push(data)}; } },
        registerProcessor(_name, Class) { Worklet = Class; },
    };
    vm.runInNewContext(source, env);
    const options = {channels:2, report:256, refillAt:128};
    if (shared) {
        options.data = new SharedArrayBuffer(8192 * 2 * 4);
        options.index = new SharedArrayBuffer(16);
    }
    const worklet = new Worklet({processorOptions:options});
    const producer = {postMessage:data=>workerReports.push(data),start(){this.started=true;}};
    worklet.port.onmessage({data:{producerPort:producer}});
    assert.equal(producer.started,true);
    assert.equal(workerReports[0].ready,true);
    assert.throws(()=>worklet.port.onmessage({data:{producerPort:producer}}), /already attached/);
    const pcm = new Float32Array(512 * 2).fill(.25);
    if (shared) {
        new Float32Array(options.data).set(pcm);
        Atomics.store(new Int32Array(options.index),1,512);
    } else {
        producer.onmessage({data:pcm});
    }
    assert.throws(()=>worklet.port.onmessage({data:pcm}), /multiple producers/);
    const channels = [[new Float32Array(128),new Float32Array(128)]];
    for (let i=0;i<8;i++) {
        worklet.process([],channels);
        env.currentTime += 128/48000;
    }
    assert.equal(worklet.consumed,512);
    assert.equal(worklet.underruns,4);
    assert.equal(mainReports.at(-1).underruns,3);
    assert.equal(workerReports.at(-1).underruns,3);
    assert.equal(workerReports.at(-1).consumed,mainReports.at(-1).consumed);
}

// If a late refill arrives when the queue is already below the threshold, it
// may never cross above that threshold. The acknowledgement must re-arm the
// request without flooding a stalled producer or concealing real underruns.
for (const shared of [false, true]) {
    let Worklet;
    const workerReports = [];
    const env = {
        sampleRate:48000, currentTime:0, Float32Array,
        AudioWorkletProcessor:class {constructor(){this.port={postMessage(){}};}},
        registerProcessor(_name, Class){Worklet=Class;},
    };
    vm.runInNewContext(source,env);
    const options={channels:2,report:2048,refillAt:1024};
    if(shared){options.data=new SharedArrayBuffer(8192*2*4);options.index=new SharedArrayBuffer(16);}
    const worklet=new Worklet({processorOptions:options});
    const producer={postMessage:data=>workerReports.push(data),start(){}};
    worklet.port.onmessage({data:{producerPort:producer}});
    let tail=0;
    function publish(frames){
        const pcm=new Float32Array(frames*2).fill(.25);
        if(shared){
            new Float32Array(options.data).set(pcm,tail*2);
            tail+=frames;
            Atomics.store(new Int32Array(options.index),1,tail);
        }else producer.onmessage({data:pcm});
    }
    const channels=[[new Float32Array(128),new Float32Array(128)]];
    function step(n){for(let i=0;i<n;i++){worklet.process([],channels);env.currentTime+=128/48000;}}
    const requests=()=>workerReports.filter(report=>report.refill).map(report=>report.refill);
    publish(1536);
    step(4);
    assert.deepEqual(requests(),[1]);
    step(4);
    assert.deepEqual(requests(),[1], 'a pending request was repeated before its acknowledgement');
    publish(512); // The queue is only back to the threshold, not above it.
    producer.onmessage({data:{refilled:1}});
    step(1);
    assert.deepEqual(requests(),[1,2], 'a late refill lost the next threshold request');
    step(16);
    assert.deepEqual(requests(),[1,2], 'a stalled producer was flooded');
    assert.ok(worklet.underruns>0, 'real starvation was hidden');
    producer.onmessage({data:{refilled:1}}); // A stale acknowledgement cannot re-arm request 2.
    step(1);
    assert.deepEqual(requests(),[1,2]);
    producer.onmessage({data:{refilled:2}});
    step(1);
    assert.deepEqual(requests(),[1,2,3]);
}

// Quiescing is explicit silence, not a counter reset: old PCM is discarded,
// new PCM waits until resume, and genuine underruns before and after remain.
for (const shared of [false, true]) {
    let Worklet;
    const pageReports=[],workerReports=[];
    const env={sampleRate:48000,currentTime:0,Float32Array,
        AudioWorkletProcessor:class {constructor(){this.port={postMessage:data=>pageReports.push(data)};}},
        registerProcessor(_name,Class){Worklet=Class;}};
    vm.runInNewContext(source,env);
    const options={channels:2,report:2048,refillAt:128};
    if(shared){options.data=new SharedArrayBuffer(8192*2*4);options.index=new SharedArrayBuffer(16);}
    const worklet=new Worklet({processorOptions:options});
    const producer={postMessage:data=>workerReports.push(data),start(){}};
    worklet.port.onmessage({data:{producerPort:producer}});
    let tail=0;
    function publish(frames,value){
        const pcm=new Float32Array(frames*2).fill(value);
        if(shared){new Float32Array(options.data).set(pcm,tail*2);tail+=frames;Atomics.store(new Int32Array(options.index),1,tail);}
        else producer.onmessage({data:pcm});
    }
    const channels=[[new Float32Array(128),new Float32Array(128)]];
    function read(){worklet.process([],channels);env.currentTime+=128/48000;return channels[0][0][0];}
    publish(128,.25);
    assert.equal(read(),.25);
    assert.equal(read(),0);
    assert.equal(worklet.underruns,1,'negative control did not record a real underrun');
    publish(512,.25);
    producer.onmessage({data:{quiesce:7}});
    assert.equal(workerReports.at(-1).quiesced,7);
    assert.equal(pageReports.at(-1).underruns,1,'quiesce lost the final worklet counters');
    assert.equal(pageReports.at(-1).quiet,true);
    assert.equal(pageReports.at(-1).quietToken,7);
    publish(512,.75);
    const consumed=worklet.consumed;
    for(let i=0;i<4;i++)assert.equal(read(),0,'quiesced output emitted PCM');
    assert.equal(worklet.consumed,consumed,'quiesced output consumed newly primed PCM');
    assert.equal(worklet.underruns,1,'intentional silence changed previous counters');
    producer.onmessage({data:{resume:8}});
    assert.equal(workerReports.at(-1).resumed,8);
    assert.equal(pageReports.at(-1).quiet,false);
    assert.equal(pageReports.at(-1).suppressed,512);
    for(let i=0;i<4;i++)assert.equal(read(),.75,'old PCM played or new PCM was lost');
    assert.equal(read(),0);
    assert.equal(worklet.underruns,2,'real starvation after resume was hidden');
}

// A replacement is quiet from its very first producer attachment. Skips and
// re-priming may finish before it consumes any output, even on a running context.
for (const shared of [false,true]) {
    let Worklet;
    const pageReports=[],workerReports=[];
    const env={sampleRate:48000,currentTime:0,Float32Array,
        AudioWorkletProcessor:class {constructor(){this.port={postMessage:data=>pageReports.push(data)};}},
        registerProcessor(_name,Class){Worklet=Class;}};
    vm.runInNewContext(source,env);
    const options={channels:2,report:2048,refillAt:128};
    if(shared){options.data=new SharedArrayBuffer(8192*2*4);options.index=new SharedArrayBuffer(16);}
    const worklet=new Worklet({processorOptions:options});
    const producer={postMessage:data=>workerReports.push(data),start(){}};
    worklet.port.onmessage({data:{producerPort:producer,quiet:true}});
    assert.equal(pageReports.at(-1).quiet,true);
    assert.equal(pageReports.at(-1).quietToken,0);
    assert.equal(workerReports.at(-1).quiet,true);
    const pcm=new Float32Array(512*2).fill(.5);
    if(shared){new Float32Array(options.data).set(pcm);Atomics.store(new Int32Array(options.index),1,512);}
    else producer.onmessage({data:pcm});
    const channels=[[new Float32Array(128),new Float32Array(128)]];
    worklet.process([],channels);
    assert.equal(channels[0][0][0],0);
    assert.equal(worklet.consumed,0);
    producer.onmessage({data:{resume:1}});
    worklet.process([],channels);
    assert.equal(channels[0][0][0],.5);
    assert.equal(worklet.consumed,128);
}
