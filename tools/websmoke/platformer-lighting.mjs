// The canonical platformer release export, with the recorded full-level input.
// This is a functional software-GPU replay/capture, not a hardware perf claim.
import fs from "node:fs";
import http from "node:http";
import path from "node:path";
import crypto from "node:crypto";
import { releaseBloom } from "./release-bloom.mjs";
import { finish, finishTick, player, releaseScenario } from "./release-scenarios.mjs";
import { PNG } from "pngjs";
const { chromium, firefox } = await import(process.env.PLAYWRIGHT_MODULE ?? "playwright-core");

const opts = Object.fromEntries(process.argv.slice(2).map(arg => {
    const at = arg.indexOf("=");
    return at < 0 ? [arg.replace(/^--/, ""), "true"] : [arg.slice(2, at), arg.slice(at + 1)];
}));
const scenario = opts.scenario ?? "lighting";
if (!["lighting", "finish", "restore", "topdown", "bloom"].includes(scenario)) throw new Error("unknown release scenario: " + scenario);
const root = path.resolve(opts.root ?? "../../.cache/lighting/platformer/web");
const out = path.resolve(opts.out ?? "../../.cache/lighting/platformer/browser");
const replayPath = path.resolve(opts.replay ?? "../../tests/integration/testdata/platformer_lighting_route.aeinput");
const route = fs.readFileSync(replayPath, "utf8");
const hash = bytes => crypto.createHash("sha256").update(bytes).digest("hex");
const manifest = fs.readFileSync(path.join(root, "export.aetext"), "utf8");
if (scenario === "topdown" && !/\n  module="topdown"/.test(manifest)) throw new Error("topdown scenario requires a topdown export");
const listing = JSON.parse(fs.readFileSync(path.join(root, "data/files.json"), "utf8"));
fs.mkdirSync(out, { recursive: true });
const report = {
    project: scenario === "topdown" ? "examples/topdown" : "examples/platformer", scenario, root, routeSHA256: scenario === "topdown" ? undefined : hash(route), seed: 42,
    manifestSHA256: hash(fs.readFileSync(path.join(root, "data/manifest.aetext"))),
    exportHash: manifest.match(/\bhash="([a-f0-9]+)"/)?.[1],
    wasmSHA256: hash(fs.readFileSync(path.join(root, "game.wasm"))),
    wasmBytes: listing.wasm, wasmGzipBytes: listing.wasmGzip,
    viewport: [1280, 720], DPR: 1, browsers: [],
    environment: "functional software GPU; paused exact-frame captures; no performance timing claim",
};

// Only the registered presentation action changes between paired replays.
// Move/Jump/Attack and all gameplay action records remain byte-identical.
function disabledRoute(source) {
    const lines = source.trimEnd().split("\n");
    const actions = lines.find(line => line.startsWith("actions ")).split(" ").slice(1);
    const index = actions.indexOf("LightingToggle");
    if (index < 0) throw new Error("canonical route has no registered LightingToggle action");
    const header = lines.filter(line => !line.startsWith("s "));
    const samples = lines.filter(line => line.startsWith("s "));
    samples.push(`s 1 ${index} 3 3f800000 0`, `s 2 ${index} 4 0 0`);
    samples.sort((a, b) => {
        const aa = a.split(" "), bb = b.split(" ");
        return +aa[1] - +bb[1] || +aa[2] - +bb[2];
    });
    return [...header, ...samples, ""].join("\n");
}

// Hold only requestAnimationFrame delivery. WebGL, its restoration events and
// the actual exported WASM keep running normally. No scene state is patched.
function exactFrames(replay) {
    const canonicalStart = typeof replay === "object";
    globalThis.aegoReplay = canonicalStart ? replay.data : replay;
    if (canonicalStart) {
        // Bench mode normally enables lockstep before the first frame, but the
        // recording starts at tick 0. Preserve the loader callback and select
        // the recorded clock before Run, then switch to lockstep at frame 1.
        let started;
        const wrapped = function (...args) {
            if (typeof started === "function") started.apply(this, args);
            if (!globalThis.aegoGame || globalThis.aegoGame.frames() !== 0) throw new Error("player startup did not expose frame 0");
            globalThis.aegoGame.lockstep(false);
        };
        Object.defineProperty(globalThis, "aegoStarted", { configurable: true, get: () => wrapped, set: value => { started = value; } });
    }
    const original = requestAnimationFrame.bind(window);
    const state = globalThis.__lightingReplay = { hold: false, held: [] };
    window.requestAnimationFrame = callback => {
        // The production player initializes at frame 1, tick 0. From there
        // request the public fixed clock for the deterministic input replay.
        if (canonicalStart && globalThis.aegoGame?.frames() === 1 && !state.firstFrame) {
            state.firstFrame = JSON.parse(aegoGame.stats());
            aegoGame.lockstep(true);
        }
        // Game.Window is signalled by Update; the actual settings menu pauses
        // Update. A menu probe stops after presentation instead.
        if (state.menuFrame && globalThis.aegoGame?.frames() >= state.menuFrame && aegoGame.menuOpen()) {
            state.hold = true;
            state.menuReady = true;
        }
        return state.hold ? (state.held.push(callback), 0) : original(callback);
    };
    const resume = () => {
        state.hold = false;
        for (const callback of state.held.splice(0)) original(callback);
    };
    state.advanceToOpenMenu = target => { state.menuFrame = target; resume(); };
    state.advance = target => {
        const game = globalThis.aegoGame;
        const from = game.frames() + 1;
        if (!game.window(from, target, () => {}, () => { state.hold = true; })) throw new Error("capture target is not ahead of the current frame");
        resume();
    };
}

const mime = { ".html": "text/html", ".js": "text/javascript", ".wasm": "application/wasm", ".json": "application/json" };
const server = http.createServer((req, res) => {
    const relative = decodeURIComponent(new URL(req.url, "http://localhost").pathname).replace(/^\/+/, "") || "index.html";
    const file = path.resolve(root, relative);
    if (!file.startsWith(root + path.sep) || !fs.existsSync(file) || !fs.statSync(file).isFile()) {
        res.writeHead(404); res.end(); return;
    }
    res.writeHead(200, { "Content-Type": mime[path.extname(file)] ?? "application/octet-stream" });
    fs.createReadStream(file).pipe(res);
});
await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));
const base = `http://127.0.0.1:${server.address().port}`;
const stats = page => page.evaluate(() => JSON.parse(globalThis.aegoGame.stats()));
async function advance(page, target, timeout = 180000) {
    await page.evaluate(t => globalThis.__lightingReplay.advance(t), target);
    await page.waitForFunction(t => globalThis.__lightingReplay.hold && globalThis.aegoGame.frames() === t, target, { timeout, polling: 100 });
}
async function capture(page, filename) {
    const bytes = await page.screenshot({ clip: { x: 0, y: 0, width: 1280, height: 720 } });
    fs.writeFileSync(path.join(out, filename), bytes);
    const png = PNG.sync.read(bytes);
    const colors = new Set();
    for (let i = 0; i < png.data.length; i += 32) colors.add(png.data.readUInt32LE(i));
    if (colors.size < 16) throw new Error(`${filename} is empty or uniform`);
    return { file: filename, pixelSHA256: hash(png.data), sampledColors: colors.size, stats: await stats(page) };
}

try {
    for (const name of (opts.browsers ?? "chromium,firefox").split(",")) {
        if (!["chromium", "firefox"].includes(name)) throw new Error(`unknown browser ${name}`);
        const isFirefox = name === "firefox";
        const result = { browser: name, passed: [], failures: [], captures: [], modes: [] };
        report.browsers.push(result);
        let browser;
        try {
            browser = await (isFirefox ? firefox : chromium).launch({
                executablePath: isFirefox ? process.env.FIREFOX : process.env.CHROMIUM ?? "/usr/bin/chromium",
                headless: !isFirefox || !process.env.DISPLAY,
                args: isFirefox ? [] : ["--no-sandbox", "--mute-audio", "--use-angle=swiftshader", "--enable-unsafe-swiftshader", "--ignore-gpu-blocklist"],
                firefoxUserPrefs: isFirefox ? { "webgl.force-enabled": true, "webgl.disabled": false } : undefined,
            });
            result.version = browser.version();
            if (scenario === "bloom") {
                await releaseBloom(browser, base, result, report, route, exactFrames, advance, capture, out);
                result.passed.push("authored Bloom around the lantern and byte-identical menu interior at matched simulation ticks");
                continue;
            }
            if (scenario !== "lighting") {
                const context = await browser.newContext({ viewport: { width: 1280, height: 720 }, deviceScaleFactor: 1 });
                try {
                    const page = await context.newPage();
                    page.on("pageerror", error => result.failures.push(error.message));
                    page.on("console", m => {
                        if (!result.console) result.console = [];
                        if (result.console.length < 300) result.console.push(m.type() + ": " + m.text().slice(0, 1500));
                    });
                    if (scenario === "finish") await page.addInitScript(exactFrames, { data: Buffer.from(route).toString("base64") });
                    await releaseScenario(scenario, page, base, result, advance, capture);
                    if (result.served["/game.wasm"] !== report.wasmSHA256 || result.served["/data/manifest.aetext"] !== report.manifestSHA256) {
                        throw new Error("browser response bytes differ from the selected release");
                    }
                    result.passed.push(scenario);
                } finally { await context.close(); }
                continue;
            }
            for (const enabled of [true, false]) {
                const mode = enabled ? "on" : "off";
                const context = await browser.newContext({ viewport: { width: 1280, height: 720 }, deviceScaleFactor: 1 });
                try {
                    const page = await context.newPage();
                    page.on("pageerror", error => result.failures.push(`${mode}: ${error.message}`));
                    page.on("console", message => {
                        if (process.env.TRACE_WEBGL) console.error(`${name} ${mode} ${message.type()}: ${message.text()}`);
                        if (message.type() === "error" || /\bERROR\b|panic:|WebGL:.*INVALID_/.test(message.text())) result.failures.push(`${mode}: ${message.text()}`);
                    });
                    page.on("requestfailed", req => result.failures.push(`${mode}: ${req.url()} ${req.failure()?.errorText}`));
                    await page.addInitScript(exactFrames, { data: Buffer.from(enabled ? route : disabledRoute(route)).toString("base64") });
                    await page.goto(`${base}/?seed=42&bench=1&observeCheckpoint=${finish}`);
                    console.log(`${name} ${mode}: export loaded; waiting for runtime`);
                    try {
                        await page.waitForFunction(() => {
                            const error = document.getElementById("error");
                            if (error && !error.hidden) throw new Error(error.textContent);
                            return globalThis.aegoGame;
                        }, null, { timeout: 120000, polling: 100 });
                    } catch (error) {
                        const state = await page.evaluate(() => ({ status: document.getElementById("status")?.textContent, error: document.getElementById("error")?.textContent, replay: globalThis.__lightingReplay, boot: globalThis.aegoBoot }));
                        throw new Error(`${error.message}; boot state ${JSON.stringify(state)}`);
                    }
                    console.log(`${name} ${mode}: runtime ready at frame ${await page.evaluate(() => globalThis.aegoGame.frames())}`);
                    await page.waitForFunction(() => globalThis.__lightingReplay.firstFrame, null, { timeout: 120000, polling: 100 });
                    const first = await page.evaluate(() => globalThis.__lightingReplay.firstFrame);
                    if (first.frames !== 1 || first.ticks !== 0) throw new Error(`canonical replay must start at frame 1, tick 0: ${JSON.stringify(first)}`);
                    await page.evaluate(() => {
                        document.getElementById("fullscreen").style.display = "none";
                        document.getElementById("mute").style.display = "none";
                    });
                    for (const frame of [400, 1039, 1216, 1840]) {
                        console.log(`${name} ${mode}: replay to ${frame}`);
                        await advance(page, frame);
                        const c = await capture(page, `${name}-${mode}-${String(frame).padStart(4, "0")}.png`);
                        if (c.stats.frames !== frame || c.stats.ticks !== frame - 1) throw new Error(`unexpected replay clock at ${frame}`);
                        const checkpoint = c.stats.checkpoint;
                        if (!checkpoint || checkpoint.node !== finish || (c.stats.ticks < finishTick
                            ? checkpoint.count !== 0 || checkpoint.tick !== 0
                            : checkpoint.count !== 1 || checkpoint.tick !== finishTick || checkpoint.by !== player)) {
                            throw new Error(`unexpected Finish checkpoint at ${frame}: ${JSON.stringify(checkpoint)}`);
                        }
                        result.renderer = c.stats.renderer;
                        result.captures.push({ lighting: enabled, frame, ...c });
                        console.log(`${name} ${mode} frame=${frame} ticks=${c.stats.ticks} pixels=${c.pixelSHA256.slice(0, 12)}`);
                    }
                    if (enabled) {
                        await page.evaluate(() => {
                            globalThis.__contextEvents = { lost: 0, restored: 0 };
                            const canvas = document.getElementById("game");
                            canvas.addEventListener("webglcontextlost", () => globalThis.__contextEvents.lost++);
                            canvas.addEventListener("webglcontextrestored", () => globalThis.__contextEvents.restored++);
                        });
                        if (!await page.evaluate(() => globalThis.aegoGame.loseContext(true))) throw new Error("WEBGL_lose_context unavailable");
                        await page.waitForFunction(() => globalThis.__contextEvents.lost === 1, null, { timeout: 10000 });
                        await page.evaluate(() => globalThis.aegoGame.loseContext(false));
                        await page.waitForFunction(() => globalThis.__contextEvents.restored === 1, null, { timeout: 30000 });
                        await advance(page, 1900);
                        result.restored = await capture(page, `${name}-restored-1900.png`);
                        if (result.restored.stats.lost || result.restored.stats.restores < 1) throw new Error("context resources did not restore");
                        result.passed.push("real context loss/restored event and continued canonical replay");
                    }
                    result.modes.push({ mode, end: await stats(page) });
                } finally { await context.close(); }
            }
            for (const frame of [400, 1039, 1216, 1840]) {
                const a = result.captures.find(c => c.frame === frame && c.lighting);
                const b = result.captures.find(c => c.frame === frame && !c.lighting);
                if (!a || !b || a.pixelSHA256 === b.pixelSHA256) throw new Error(`lighting did not change pixels at frame ${frame}`);
            }
            result.passed.push("same exported assets and gameplay input with registered LightingToggle presentation action");
            result.passed.push("exact 400/1039/1216/1840 native-compatible replay clocks and nonempty rendered images");
        } catch (error) {
            result.failures.push(error.stack ?? String(error));
            console.error(`${name}: ${error.stack ?? error}`);
        } finally {
            await browser?.close();
            console.log(`${name} ${scenario}: ${result.failures.length ? "FAIL" : "PASS"}`);
            fs.writeFileSync(path.join(out, "results.json"), JSON.stringify(report, null, 2) + "\n");
        }
    }
} finally {
    await new Promise(resolve => server.close(resolve));
    fs.writeFileSync(path.join(out, "results.json"), JSON.stringify(report, null, 2) + "\n");
}
if (report.browsers.some(browser => browser.failures.length)) process.exitCode = 1;
