import assert from "node:assert/strict";
import { test } from "node:test";
import { pauseAtMaterialFlash } from "./try-pulse.mjs";

function setup(t, value = 0.1) {
    const old = globalThis.aegoDocs;
    const state = { running: true, paused: false, frames: 12, restarts: 0 };
    const calls = [];
    const sample = { value };
    globalThis.aegoDocs = { call(method, body) {
        calls.push(method);
        if (method === "state") return JSON.stringify({ state: { ...state } });
        if (method === "play") {
            state.paused = !state.paused;
            sample.onPause?.(state);
            return JSON.stringify({ state: { ...state } });
        }
        if (method === "inspect") {
            assert.deepEqual(JSON.parse(body), { component: "engine.MaterialOverride", fields: ["engine.MaterialOverride.FlashValue"] });
            if (sample.error) return JSON.stringify({ error: sample.error });
            return JSON.stringify({ inspection: { runtimeExecuting: state.running && !state.paused,
                observables: { "engine.MaterialOverride.FlashValue": sample.value } } });
        }
        throw new Error(`unexpected call ${method}`);
    } };
    t.after(() => { if (old === undefined) delete globalThis.aegoDocs; else globalThis.aegoDocs = old; });
    return { state, sample, calls };
}

test("pulse sampler waits for real low and high values before pausing", t => {
    const { state, sample, calls } = setup(t, 0.5);
    assert.equal(pauseAtMaterialFlash({ relation: "atMost", threshold: 0.25 }), false);
    assert.deepEqual(calls, ["state", "inspect"]);
    sample.value = 0.2;
    assert.deepEqual(pauseAtMaterialFlash({ relation: "atMost", threshold: 0.25 }), { value: 0.2, frames: 12, restarts: 0 });
    assert.equal(state.paused, true);
    state.paused = false;
    state.frames = 40;
    sample.value = 0.8;
    assert.deepEqual(pauseAtMaterialFlash({ relation: "atLeast", threshold: 0.75 }), { value: 0.8, frames: 40, restarts: 0 });
    assert.equal(state.paused, true);
});

test("pulse sampler fails closed on errors, invalid values and stopped scenes", t => {
    const { state, sample } = setup(t);
    assert.throws(() => pauseAtMaterialFlash({ relation: "unknown", threshold: 0.5 }), /invalid/);
    assert.throws(() => pauseAtMaterialFlash({ relation: "atMost", threshold: NaN }), /invalid/);
    state.running = false;
    assert.throws(() => pauseAtMaterialFlash({ relation: "atMost", threshold: 0.25 }), /stopped/);
    state.running = true;
    state.frames = 0;
    assert.equal(pauseAtMaterialFlash({ relation: "atMost", threshold: 0.25 }), false);
    state.frames = 1;
    sample.error = "unavailable";
    assert.throws(() => pauseAtMaterialFlash({ relation: "atMost", threshold: 0.25 }), /unavailable/);
    delete sample.error;
    sample.value = null;
    assert.throws(() => pauseAtMaterialFlash({ relation: "atMost", threshold: 0.25 }), /no running Flash value/);
});

test("pulse sampler rejects a changed frame or value during pause", t => {
    const { state, sample } = setup(t);
    sample.onPause = s => { s.frames++; };
    assert.throws(() => pauseAtMaterialFlash({ relation: "atMost", threshold: 0.25 }), /changed while/);
    state.paused = false;
    sample.onPause = () => { sample.value = 0.3; };
    assert.throws(() => pauseAtMaterialFlash({ relation: "atMost", threshold: 0.25 }), /changed while/);
});
