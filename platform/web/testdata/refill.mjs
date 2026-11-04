import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";

const source = fs.readFileSync(new URL("../worklet.js", import.meta.url), "utf8");
for (const shared of [false, true]) {
    const messages = [];
    let Worklet;
    const env = {
        sampleRate: 48000, currentTime: 0,
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
