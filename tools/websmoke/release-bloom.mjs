// Pixel comparison on the exported platformer; no source server or web/GL patches.
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { PNG } from "pngjs";
import { installReleaseHashes, readReleaseHashes } from "./release-scenarios.mjs";

function menuRecording(route) {
    const lines = route.trimEnd().split("\n");
    const index = lines.find(line => line.startsWith("actions ")).split(" ").slice(1).indexOf("Pause");
    assert(index >= 0, "recording does not contain the project's Pause action");
    const samples = lines.filter(line => line.startsWith("s ")).map(line => line.split(" "));
    assert(!samples.some(s => Number(s[2]) === index && Number(s[1]) <= 83), "fixture already contains a Pause here");
    // Leave all gameplay samples in the canonical playback; add one real button press
    // after the world screenshot. The game's own player script opens its settings.
    samples.push(["s", "79", String(index), "3", "3f800000", "0"], ["s", "80", String(index), "4", "0", "0"]);
    samples.sort((a, b) => Number(a[1]) - Number(b[1]) || Number(a[2]) - Number(b[2]));
    return [...lines.filter(l => !l.startsWith("s ")), ...samples.map(s => s.join(" ")), ""].join("\n");
}

const viewport = { width: 1280, height: 720 };
const menu = [226, 66, 1054, 654];
// The native production replay projects the visible sprite into this conservative
// box at tick 75. Test for light in the surrounding ring, outside the sprite box.
const lantern = [666, 446, 678, 479];
const haloRing = [659, 441, 685, 484];
const inside = (rect, x, y) => x >= rect[0] && x < rect[2] && y >= rect[1] && y < rect[3];

function compareScreens(on, off) {
    let halo = 0, covered = 0;
    for (let y = 0; y < viewport.height; y++) {
        for (let x = 0; x < viewport.width; x++) {
            const i = 4 * (y * viewport.width + x);
            const a = on.world.data, b = off.world.data;
            if (inside(haloRing, x, y) && !inside(lantern, x, y) && a[i + 3] === b[i + 3] &&
                Math.max(a[i] - b[i], a[i + 1] - b[i + 1], a[i + 2] - b[i + 2]) > 1) halo++;
            if (!inside(menu, x, y)) continue;
            const u = on.menu.data.readUInt32BE(i), v = off.menu.data.readUInt32BE(i);
            assert.equal(u, v, `Bloom changed the menu at (${x},${y})`);
            if (a.readUInt32BE(i) !== b.readUInt32BE(i) && b.readUInt32BE(i) !== v) covered++;
        }
    }
    assert(halo > 0, "no halo detected outside the lantern coverage box");
    assert(covered > 0, "menu sample did not cover any world pixels changed by Bloom");
    return { haloPixels: halo, menuCoveredPixels: covered, haloRing, lantern, menu };
}

export async function releaseBloom(browser, base, result, report, route, exactFrames, advance, capture, out) {
    const input = menuRecording(route);
    result.probeReplaySHA256 = createHash("sha256").update(input).digest("hex");
    const pictures = {};
    for (const mode of ["off", "on"]) {
        const context = await browser.newContext({ viewport, deviceScaleFactor: 1 });
        try {
            const page = await context.newPage();
            const errors = [];
            page.on("pageerror", err => errors.push(err.message));
            page.on("requestfailed", req => errors.push(`${req.url()}: ${req.failure()?.errorText}`));
            await page.addInitScript(exactFrames, { data: Buffer.from(input).toString("base64") });
            await installReleaseHashes(page);
            const setting = "render2d.postProcessing", enabled = mode === "on";
            const query = new URLSearchParams({seed:"42", bench:"1", benchBool:setting+":"+(enabled?"1":"0")});
            await page.goto(`${base}/?${query}`);
            await page.waitForFunction(() => globalThis.aegoGame?.frames() >= 2, null, { polling: 50, timeout: 120000 });
            assert.equal(await page.evaluate(k => aegoGame.boolSetting(k), setting), enabled);
            const served = await readReleaseHashes(page);
            assert.equal(served["/game.wasm"], report.wasmSHA256);
            assert.equal(served["/data/manifest.aetext"], report.manifestSHA256);
            const canvas = await page.locator("#game").evaluate(c => {
                const r = c.getBoundingClientRect();
                return [c.width,c.height,r.x,r.y,r.width,r.height,devicePixelRatio];
            });
            assert.deepEqual(canvas,[1280,720,0,0,1280,720,1],"canvas geometry changed");
            const initial = await page.evaluate(() => JSON.parse(aegoGame.stats()));
            const offset = initial.frames - initial.ticks;
            assert([0,1].includes(offset), "unexpected startup tick offset");
            const snap = async (name, frame) => {
                if (name === "menu") {
                    await page.evaluate(target => globalThis.__lightingReplay.advanceToOpenMenu(target), frame);
                    try {
                        await page.waitForFunction(() => globalThis.__lightingReplay.menuReady, null, {polling:50,timeout:60000});
                    } catch (err) {
                        const state = await page.evaluate(() => ({menuOpen:aegoGame.menuOpen(), frame:aegoGame.frames(),
                            tick:JSON.parse(aegoGame.stats()).ticks, held:globalThis.__lightingReplay.hold}));
                        throw new Error(`presentation did not reach the paused menu: ${JSON.stringify(state)}`, {cause:err});
                    }
                } else {
                    await advance(page,frame);
                }
                const shot = await capture(page,`${result.browser}-bloom-${mode}-${name}.png`);
                assert.equal(await page.evaluate(() => aegoGame.menuOpen()), name === "menu");
                assert.equal(shot.stats.postFXFailures,0);
                assert.equal(shot.stats.postFXPasses > 0,enabled);
                const png = PNG.sync.read(fs.readFileSync(path.join(out,shot.file)));
                assert.equal(png.width,viewport.width);assert.equal(png.height,viewport.height);
                result.captures.push({mode,stage:name,served,...shot});
                console.log(`${result.browser} Bloom ${mode}, ${name}: frame ${shot.stats.frames}, tick ${shot.stats.ticks}, ${shot.stats.postFXPasses} passes`);
                return {png, tick:shot.stats.ticks};
            };
            const world = await snap("world",75+offset);
            assert.equal(world.tick,75);
            const ui = await snap("menu",83+offset);
            assert.deepEqual(errors,[]);
            pictures[mode] = {world:world.png,menu:ui.png,menuTick:ui.tick};
            result.renderer = initial.renderer;
        } finally {await context.close();}
    }
    assert.equal(pictures.on.menuTick,pictures.off.menuTick,"opening the menu changed the tick differently across profiles");
    result.bloom = {tick:75,menuTick:pictures.on.menuTick,...compareScreens(pictures.on,pictures.off)};
}
