import assert from "node:assert/strict";
import test from "node:test";
import {neutralCapturePoint, parkCapturePointer, sameCaptureViewport} from "./try-capture.mjs";

const viewport = {x: 0, y: 28, width: 292, height: 530, canvasWidth: 688, canvasHeight: 560};
const bounds = {x: 13, y: 17, width: 344, height: 280};

test("parks within the canvas, outside the game, respecting CSS scale", () => {
    assert.deepEqual(neutralCapturePoint(viewport, bounds), {x: 345, y: 29});
});

test("falls back if the preferred canvas corner lies inside Game View", () => {
    const right = {...viewport, x: 400, y: 0, width: 288, height: 200};
    assert.deepEqual(neutralCapturePoint(right, bounds), {x: 25, y: 29});
});

test("cannot park in the Game View or with invalid bounds", () => {
    for (const bad of [null, {...viewport, width: -1}, {...viewport, width: 689},
        {...viewport, y: NaN}, {x: 0, y: 0, width: 688, height: 560, canvasWidth: 688, canvasHeight: 560}]) {
        assert.throws(() => neutralCapturePoint(bad, bounds));
    }
    assert.throws(() => neutralCapturePoint(viewport, {...bounds, height: 0}));
});

test("detects a changed capture layout", () => {
    assert.equal(sameCaptureViewport(viewport, {...viewport}), true);
    assert.equal(sameCaptureViewport(viewport, {...viewport, y: 29}), false);
    assert.equal(sameCaptureViewport(viewport, null), false);
});

test("moves the pointer only after verifying it hits the canvas", async () => {
    const calls = [];
    const page = {
        locator(selector) { assert.equal(selector, ".try canvas"); return {
            async scrollIntoViewIfNeeded() { calls.push("scroll"); },
            async boundingBox() { calls.push("bounds"); return bounds; },
        }; },
        async evaluate(_fn, point) { calls.push(["hit", point]); return true; },
        mouse: {async move(x, y) { calls.push(["move", x, y]); }},
    };
    await parkCapturePointer(page, viewport);
    assert.deepEqual(calls, ["scroll", "bounds", ["hit", {x: 345, y: 29}], ["move", 345, 29]]);
    page.evaluate = async () => false;
    await assert.rejects(parkCapturePointer(page, viewport), /not on the canvas/);
    assert.equal(calls.filter(c => Array.isArray(c) && c[0] === "move").length, 1);
});
