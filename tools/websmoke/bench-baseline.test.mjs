import assert from "node:assert/strict";
import { test } from "node:test";
import { requiredCIBaselineErrors, requiredCIGateOptionErrors, requiredCIMeasurementErrors } from "./bench-baseline.mjs";

const config = { gpu: "swiftshader", warmup: 120, frames: 600, dpr: 1, maxdpr: null };
function valid() {
    return {
        meta: { gpu: "swiftshader", viewport: "1280x720", dpr: 1, maxdpr: null,
                warmup: 120, frames: 600, measurementOnly: false },
        results: { chromium: { browser: "chromium",
            runs: Array.from({length: 5}, () => ({frames: 600, renderer: "ANGLE SwiftShader", errors: []})),
            summary: { drawsPerFrame: 19, frameBytesPerFrame: 367,
                bridge: {calls: 250, draws: 19, bufferUploads: 8, bufferKB: 3,
                         texUploads: 0, texKB: 0, programs: 15, targets: 7},
                samples: {gaps: [16.7], work: [2]} },
        } },
    };
}

test("a complete CI benchmark baseline is accepted", () => {
    assert.deepEqual(requiredCIBaselineErrors(valid(), ["chromium"], config), []);
});

test("missing, incompatible and partial baselines fail closed", () => {
    assert(requiredCIBaselineErrors(null, ["chromium"], config).length > 0);
    const wrongMode = valid(); wrongMode.meta.gpu = "hardware";
    assert(requiredCIBaselineErrors(wrongMode, ["chromium"], config).length > 0);
    const missingBrowser = valid(); delete missingBrowser.results.chromium;
    assert(requiredCIBaselineErrors(missingBrowser, ["chromium"], config).length > 0);
    const tooFew = valid(); tooFew.results.chromium.runs.pop();
    assert(requiredCIBaselineErrors(tooFew, ["chromium"], config).length > 0);
    const wrongRenderer = valid(); wrongRenderer.results.chromium.runs[0].renderer = "Intel";
    assert(requiredCIBaselineErrors(wrongRenderer, ["chromium"], config).length > 0);
    const missingCounter = valid(); delete missingCounter.results.chromium.summary.bridge.draws;
    assert(requiredCIBaselineErrors(missingCounter, ["chromium"], config).length > 0);
    const invalidBytes = valid(); invalidBytes.results.chromium.summary.frameBytesPerFrame = NaN;
    assert(requiredCIBaselineErrors(invalidBytes, ["chromium"], config).length > 0);
    const missingSamples = valid(); missingSamples.results.chromium.summary.samples.work = [];
    assert(requiredCIBaselineErrors(missingSamples, ["chromium"], config).length > 0);
    assert(requiredCIBaselineErrors(valid(), ["chromium"], {...config, frames: 601}).length > 0);
});

test("CI cannot update its baseline, disable profiling, or omit all browsers", () => {
    assert.deepEqual(requiredCIGateOptionErrors({ browsers: "chromium" }), []);
    assert(requiredCIGateOptionErrors({ "update-baseline": "true" }).length > 0);
    assert(requiredCIGateOptionErrors({ profile: "false" }).length > 0);
    assert(requiredCIGateOptionErrors({ browsers: "" }).length > 0);
});

test("CI requires a complete measurement and profile on the requested renderer", () => {
    const actual = valid().results;
    actual.chromium.summary.allocs = {frames: 600, frameObjects: 0, sites: []};
    const options = {gpu: "swiftshader", frames: 600, runs: 5};
    assert.deepEqual(requiredCIMeasurementErrors(actual, ["chromium"], options), []);
    assert(requiredCIMeasurementErrors({}, ["chromium"], options).length > 0);
    const partial = structuredClone(actual); partial.chromium.runs.pop();
    assert(requiredCIMeasurementErrors(partial, ["chromium"], options).length > 0);
    const wrongRenderer = structuredClone(actual); wrongRenderer.chromium.runs[0].renderer = "Intel";
    assert(requiredCIMeasurementErrors(wrongRenderer, ["chromium"], options).length > 0);
    const wrongFrames = structuredClone(actual); wrongFrames.chromium.runs[0].frames = 599;
    assert(requiredCIMeasurementErrors(wrongFrames, ["chromium"], options).length > 0);
    const missingMetric = structuredClone(actual); delete missingMetric.chromium.summary.frameBytesPerFrame;
    assert(requiredCIMeasurementErrors(missingMetric, ["chromium"], options).length > 0);
    const missingProfile = structuredClone(actual); delete missingProfile.chromium.summary.allocs;
    assert(requiredCIMeasurementErrors(missingProfile, ["chromium"], options).length > 0);
    const badProfile = structuredClone(actual); badProfile.chromium.summary.allocs.frames = 599;
    assert(requiredCIMeasurementErrors(badProfile, ["chromium"], options).length > 0);
});
