// Independent checks on the same standalone platformer export.
import assert from "node:assert/strict";

export const finish = "fb3a738501b6010f2a581840c6194e41";
export const player = "c0000000000000000000000000000003";
export const finishTick = 1810;
const stats = page => page.evaluate(() => JSON.parse(globalThis.aegoGame.stats()));
const wait = (page, fn, arg) => page.waitForFunction(fn, arg, { polling: 100, timeout: 60000 });

export async function installReleaseHashes(page) {
    await page.addInitScript(() => {
        const original = window.fetch;
        const hashes = globalThis.__releaseHashes = {};
        window.fetch = async function (...args) {
            const response = await original.apply(this, args);
            const key = new URL(response.url).pathname;
            if (response.ok && ["/game.wasm", "/data/manifest.aetext"].includes(key)) {
                // DevTools can evict a large WASM body by the time boot completes.
                // Hash a clone of the very response consumed by the game loader.
                hashes[key] = response.clone().arrayBuffer().then(async buffer => {
                    const hash = new Uint8Array(await crypto.subtle.digest("SHA-256", buffer));
                    return Array.from(hash, n => n.toString(16).padStart(2, "0")).join("");
                });
            }
            return response;
        };
    });
}

export async function readReleaseHashes(page) {
    return page.evaluate(async () => {
        const out = {};
        for (const k of ["/game.wasm", "/data/manifest.aetext"]) {
            if (!globalThis.__releaseHashes[k]) throw new Error("missing release fetch: " + k);
            out[k] = await globalThis.__releaseHashes[k];
        }
        return out;
    });
}

export async function releaseScenario(name, page, base, result, advance, capture) {
    const query = name === "finish" ? `?seed=42&observeCheckpoint=${finish}` : "?seed=42";
    await installReleaseHashes(page);
    await page.goto(base + "/" + query);
    await wait(page, () => {
        const error = document.querySelector("#error");
        if (error && !error.hidden) throw new Error(error.textContent);
        return globalThis.aegoGame?.frames() >= 2;
    });
    result.served = await readReleaseHashes(page);
    result.renderer = (await stats(page)).renderer;
    if (name === "finish") {
        const first = await page.evaluate(() => globalThis.__lightingReplay.firstFrame);
        assert(first && first.frames === 1 && first.ticks === 0, "canonical startup must be zero-delta");
        result.samples = [{ frame: first.frames, tick: first.ticks, checkpoint: first.checkpoint }];
        for (const frame of [75, 213, 224, 260, finishTick, finishTick + 2, 2400]) {
            await advance(page, frame, 1200000);
            const s = await stats(page);
            result.samples.push({ frame: s.frames, tick: s.ticks, checkpoint: s.checkpoint, postFXFailures: s.postFXFailures });
            console.log(`${result.browser}: canonical replay frame ${s.frames}, tick ${s.ticks}, Finish signals ${s.checkpoint?.count}`);
            assert.equal(s.frames, frame);
            assert.equal(s.ticks, frame - 1);
            assert.equal(s.postFXFailures, 0);
            if (s.ticks < finishTick) assert.deepEqual(s.checkpoint, { node: finish, tick: 0, count: 0 });
            else assert.deepEqual(s.checkpoint, { node: finish, by: player, tick: finishTick, count: 1 });
        }
    } else if (name === "topdown") {
        const before = await stats(page);
        await page.locator("canvas[data-aego]").focus();
        await page.keyboard.down("KeyD");
        try { await wait(page, frame => globalThis.aegoGame.frames() >= frame + 30, before.frames); }
        finally { await page.keyboard.up("KeyD"); }
        await page.keyboard.press("KeyJ");
        await wait(page, keys => JSON.parse(globalThis.aegoGame.stats()).keys >= keys + 4, before.keys);
        const final = await stats(page);
        assert(final.ticks > before.ticks && final.draws > before.draws, "topdown did not simulate and render");
        assert.equal(final.postFXFailures, 0);
        result.topdown = { before, final, capture: await capture(page, `${result.browser}-topdown.png`) };
    } else if (name === "restore") {
        await page.evaluate(() => {
            const canvas = document.querySelector("canvas[data-aego]");
            globalThis.__releaseGL = [];
            for (const event of ["webglcontextlost", "webglcontextrestored"]) {
                canvas.addEventListener(event, e => globalThis.__releaseGL.push({ type: e.type, trusted: e.isTrusted }));
            }
        });
        const before = await stats(page);
        assert(await page.evaluate(() => globalThis.aegoGame.loseContext(true)), "WEBGL_lose_context absent");
        await wait(page, () => globalThis.__releaseGL.some(e => e.type === "webglcontextlost" && e.trusted));
        await page.evaluate(() => globalThis.aegoGame.loseContext(false));
        await wait(page, () => globalThis.__releaseGL.some(e => e.type === "webglcontextrestored" && e.trusted));
        const after = await stats(page);
        await wait(page, frame => globalThis.aegoGame.frames() >= frame + 10, after.frames);
        const final = await stats(page);
        assert.equal(final.lost, false);
        assert.equal(final.restores, before.restores + 1);
        assert.equal(final.postFXFailures, before.postFXFailures);
        assert(final.postFXPasses > after.postFXPasses, "world PostFX stopped after restore");
        result.restore = { before, final, events: await page.evaluate(() => globalThis.__releaseGL),
            capture: await capture(page, `${result.browser}-restored.png`) };
    }
}
