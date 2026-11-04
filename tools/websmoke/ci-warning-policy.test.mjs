import assert from "node:assert/strict";
import { test } from "node:test";
import { BrowserEventTimeout, frameLimitOutcome, visibilityFailureOutcome } from "./ci-warning-policy.mjs";

test("frame limit keeps both bounds and stays strict outside CI", () => {
    for (const fps of [26, 29.9, 32]) assert.equal(frameLimitOutcome(fps, "SwiftShader", false), "pass");
    assert.equal(frameLimitOutcome(16.3, "SwiftShader", false), "fail");
    assert.equal(frameLimitOutcome(32.1, "SwiftShader", true), "fail");
});

test("only low FPS on verified SwiftShader is a CI warning", () => {
    assert.equal(frameLimitOutcome(16.3, "ANGLE (Google, SwiftShader Device)", true), "warning");
    assert.equal(frameLimitOutcome(16.3, "Intel Iris Xe", true), "fail");
    assert.equal(frameLimitOutcome(16.3, "unknown", true), "fail");
    assert.equal(frameLimitOutcome(NaN, "SwiftShader", true), "fail");
});

test("only a Firefox setup timeout is a CI warning", () => {
    const ready = new BrowserEventTimeout("ready");
    assert.equal(visibilityFailureOutcome("firefox", ready, true, false), "warning");
    assert.equal(visibilityFailureOutcome("firefox", ready, false, false), "fail");
    assert.equal(visibilityFailureOutcome("chromium", ready, true, false), "fail");
    for (const kind of ["hidden", "resumed"]) {
        assert.equal(visibilityFailureOutcome("firefox", new BrowserEventTimeout(kind), true, false), "warning");
    }
    for (const kind of ["hidden", "visible", "resumed"]) {
        assert.equal(visibilityFailureOutcome("firefox", new BrowserEventTimeout(kind), true, true), "fail");
    }
    assert.equal(visibilityFailureOutcome("firefox", new Error(ready.message), true, false), "fail");
});
