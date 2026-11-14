import fs from "node:fs";
import http from "node:http";
import path from "node:path";
import { chromium } from "playwright-core";

const opts = Object.fromEntries(
    process.argv.slice(2).map((a) => {
        const [k, v] = a.replace(/^--/, "").split("=");
        return [k, v ?? "true"];
    }),
);
const site = path.resolve(opts.site ?? "../../site");
const executablePath = process.env.CHROMIUM || "/usr/bin/chromium";
const seconds = +(opts.seconds ?? 10);
const demoSeconds = +(opts["demo-seconds"] ?? 2.5);
if (!Number.isFinite(demoSeconds) || demoSeconds < 2.5) throw new Error("demo-seconds must be at least 2.5");
const blocks = (opts.blocks ?? "").split(",").filter(Boolean).map(Number);
const mime = {
    ".html": "text/html",
    ".js": "text/javascript",
    ".css": "text/css",
    ".wasm": "application/wasm",
    ".json": "application/json",
};
const isolate = {
    "Cross-Origin-Opener-Policy": "same-origin",
    "Cross-Origin-Embedder-Policy": "require-corp",
};
const failures = [];

function fail(msg) {
    failures.push(msg);
    console.error("FAIL " + msg);
}

function sleep(ms) {
    return new Promise((r) => setTimeout(r, ms));
}

// Playwright evaluate/waitForFunction can synthesize gestures; audio/autoplay probes must not activate the page before real pointer input, even during startup.
async function scriptWithoutGesture(page) {
    const cdp = await page.context().newCDPSession(page);
    const js = async (expression) => {
        const r = await cdp.send("Runtime.evaluate", { expression, returnByValue: true, awaitPromise: true, userGesture: false });
        if (r.exceptionDetails) throw new Error(r.exceptionDetails.exception?.description ?? r.exceptionDetails.text);
        return r.result.value;
    };
    const until = async (expression, timeout) => {
        const end = Date.now() + timeout;
        while (!(await js(expression))) {
            if (Date.now() > end) throw new Error("timed out waiting for " + expression);
            await sleep(100);
        }
    };
    return { js, until };
}

function serve(root, headers) {
    return new Promise((resolve) => {
        const srv = http.createServer((req, res) => {
            let p = decodeURIComponent(new URL(req.url, "http://local").pathname);
            if (p.endsWith("/")) p += "index.html";
            const file = path.join(root, p);
            if (!file.startsWith(root) || !fs.existsSync(file) || fs.statSync(file).isDirectory()) {
                res.writeHead(404);
                res.end();
                return;
            }
            res.writeHead(200, { "Content-Type": mime[path.extname(file)] || "application/octet-stream", ...headers });
            fs.createReadStream(file).pipe(res);
        });
        srv.listen(0, "127.0.0.1", () => resolve(srv));
    });
}

async function run(browser, label, headers, wantShared, depth = 0) {
    const srv = await serve(site, headers);
    const page = await browser.newPage({ viewport: { width: 1280, height: 800 }, deviceScaleFactor: 1 });
    page.on("pageerror", (e) => fail(`${label}: page error: ${e.message}`));
    const { js, until } = await scriptWithoutGesture(page);
    const stats = async () => JSON.parse(await js("globalThis.aegoAudio.stats()"));
    try {
        try { await page.addInitScript(installAudioTiming); }
        catch { /* Timing is diagnostic only; keep the audio assertions strict. */ }
        await page.goto(`http://127.0.0.1:${srv.address().port}/play/`);
        await until("!!(globalThis.aegoAudio && globalThis.aegoAudio.start)", 120000);
        const isolated = await js("globalThis.crossOriginIsolated === true");
        if (isolated !== wantShared) fail(`${label}: crossOriginIsolated is ${isolated}`);
        const enforced = await js("(async () => { const c = new AudioContext(); await new Promise((r) => setTimeout(r, 300)); const s = c.state; await c.close(); return s !== 'running'; })()");
        if (depth) await js(`globalThis.aegoAudioBlocks = ${depth}`);
        const err = await js("globalThis.aegoAudio.start()");
        if (err) throw new Error("start: " + err);
        await sleep(500);
        const before = await stats();
        if (!enforced) console.log(`${label}: this browser starts an AudioContext without a gesture, the gesture check is skipped`);
        else if (before.running || before.queued !== 0) fail(`${label}: audio ran before a gesture: ${JSON.stringify(before)}`);
        await page.mouse.click(5, 5);
        await until("JSON.parse(globalThis.aegoAudio.stats()).running", 10000);
        await sleep(seconds * 1000);
        const s = await stats();
        if (s.error) fail(`${label}: ${s.error}`);
        if (s.shared !== wantShared) fail(`${label}: shared transport is ${s.shared}`);
        if (Math.abs(s.rms - 0.354) > 0.05) fail(`${label}: rms ${s.rms}`);
        if (!Number.isInteger(s.targetBlocks) || s.targetBlocks <= 0 || !Number.isInteger(s.blockFrames) || s.blockFrames <= 0) {
            throw new Error(`invalid actual audio target: ${JSON.stringify(s)}`);
        }
        const used = s.targetBlocks;
        if (depth && used !== depth) fail(`${label}: requested ${depth} blocks but the pump uses ${used}`);
        console.log(`${label}: rate ${s.rate}, shared ${s.shared}, ${used} blocks (${((used * s.blockFrames * 1000) / s.rate).toFixed(0)} ms), underruns ${s.underruns} (pump ${s.pumpUnderruns}) in ${seconds} s, rms ${s.rms.toFixed(3)}, mix max ${s.mixMaxUs} us`);
        if (!Number.isInteger(s.pumpUnderruns) || s.pumpUnderruns < 0) fail(`${label}: invalid pump underrun count`);
        if (!Number.isInteger(s.underruns) || s.underruns < 0) fail(`${label}: invalid worklet underrun count`);
        if (s.underruns > 0) fail(`${label}: ${s.underruns} underruns in ${seconds} s at ${used} blocks`);
        if (s.pumpUnderruns > 0) fail(`${label}: ${s.pumpUnderruns} predicted pump underruns in ${seconds} s at ${used} blocks`);
        if (s.underruns > 0 || s.pumpUnderruns > 0) {
            console.log(`${label}: before-click audio counters (diagnostic only): ${JSON.stringify({
                running: before.running, queued: before.queued, underruns: before.underruns, pumpUnderruns: before.pumpUnderruns,
            })}`);
            try {
                const timing = await js("JSON.stringify({now:performance.now(), timing:globalThis.__aegoAudioSmokeTiming ?? null})");
                console.log(`${label}: underrun timing (main-thread long tasks and timer gaps, ms since navigation; diagnostic only): ${timing}`);
            } catch {
                console.log(`${label}: underrun timing unavailable`);
            }
        }
        await js("globalThis.aegoAudio.stop()");
    } catch (e) {
        fail(`${label}: ${e.message}`);
    } finally {
        await page.close();
        srv.close();
    }
}

// Installed before the tone/demo loads. Long tasks and timer gaps are main-thread
// diagnostics, not proof of an audio stall's cause.
function installAudioTiming() {
    const timing = {
        available: false, count: 0, maxMS: 0, tasks: [],
        heartbeatAvailable: false, gapCount: 0, maxGapMS: 0, gaps: [],
        unmuteMS: null, beforeClickSampleMS: null, firstClickMS: null,
    };
    globalThis.__aegoAudioSmokeTiming = timing;
    window.addEventListener("pointerdown", (e) => {
        if (e.isTrusted && timing.firstClickMS === null) timing.firstClickMS = performance.now();
    }, true);
    const retain = (items, item) => {
        // Keep the first and last 16 entries, even if startup is busy.
        if (items.length === 32) items.splice(16, 1);
        items.push(item);
    };
    try {
        let last = performance.now();
        setInterval(() => {
            const now = performance.now();
            const gap = now - last;
            last = now;
            if (gap < 100) return;
            timing.gapCount++;
            timing.maxGapMS = Math.max(timing.maxGapMS, gap);
            retain(timing.gaps, { atMS: now, gapMS: gap });
        }, 50);
        timing.heartbeatAvailable = true;
    } catch {
        // Continue without timer diagnostics.
    }
    try {
        if (!globalThis.PerformanceObserver?.supportedEntryTypes?.includes("longtask")) return;
        const observer = new PerformanceObserver((list) => {
            for (const entry of list.getEntries()) {
                const task = { startMS: entry.startTime, durationMS: entry.duration };
                timing.count++;
                timing.maxMS = Math.max(timing.maxMS, task.durationMS);
                retain(timing.tasks, task);
            }
        });
        observer.observe({ type: "longtask", buffered: true });
        timing.available = true;
    } catch {
        // Lack of optional diagnostics must not change the audio assertions.
    }
}

function checkAudibleWorklet(stats, label) {
    if (stats.quiet !== false) fail(`${label}: worklet is intentionally silent while the game should be audible`);
    if (stats.backend !== "worker") fail(`${label}: demo is not using the audio Worker`);
}

async function demo(browser, label, headers, wantShared) {
    if (!fs.existsSync(path.join(site, "demo", "index.html"))) {
        fail(`${label}: site/demo is missing`);
        return;
    }
    const srv = await serve(site, headers);
    const page = await browser.newPage({ viewport: { width: 1280, height: 720 }, deviceScaleFactor: 1 });
    page.on("pageerror", (e) => fail(`${label}: page error: ${e.message}`));
    const { js, until } = await scriptWithoutGesture(page);
    const stats = async () => JSON.parse(await js("globalThis.aegoAudioStats()"));
    const base = `http://127.0.0.1:${srv.address().port}/demo/`;
    try {
        try { await page.addInitScript(installAudioTiming); }
        catch { /* The audio gate remains strict if timing diagnostics are unavailable. */ }
        await page.goto(base);
        await until("globalThis.aegoBoot !== undefined && typeof globalThis.aegoAudioStats === 'function'", 120000);
        const quiet = await js("document.documentElement.dataset.sound === 'off'");
        if (quiet && !(await stats()).muted) fail(`${label}: the page asks for silence, but the demo starts with sound`);
        await js("(globalThis.__aegoAudioSmokeTiming && (globalThis.__aegoAudioSmokeTiming.unmuteMS = performance.now()), globalThis.aegoAudioMute(false))");
        const enforced = await js(`(async () => {
            const c = new AudioContext();
            await new Promise((r) => setTimeout(r, 300));
            const s = c.state;
            await c.close();
            return s !== "running";
        })()`);
        const before = await stats();
        const activated = await js("(globalThis.__aegoAudioSmokeTiming && (globalThis.__aegoAudioSmokeTiming.beforeClickSampleMS = performance.now()), navigator.userActivation && navigator.userActivation.hasBeenActive)");
        if (activated) fail(`${label}: the harness activated the page before the real click`);
        console.log(`${label}: before click running ${before.running}, underruns ${before.underruns}, consumed ${before.consumed}, page activated ${!!activated}`);
        if (!enforced) console.log(`${label}: this browser starts an AudioContext without a gesture, the gesture check is skipped`);
        else if (before.running) fail(`${label}: audio ran before a gesture`);
        await page.click("#game");
        await sleep(demoSeconds * 1000);
        const s = await stats();
        if (s.error) fail(`${label}: ${s.error}`);
        if (s.shared !== wantShared) fail(`${label}: shared transport is ${s.shared}`);
        if (!Number.isInteger(s.underruns) || s.underruns < 0) fail(`${label}: invalid worklet underrun count`);
        const pump = await js("globalThis.aegoGame?.mixStats?.() ?? null");
        if (!pump || !Number.isInteger(pump.underruns) || pump.underruns < 0) fail(`${label}: invalid game pump underrun count`);
        else if (pump.underruns > 0) fail(`${label}: ${pump.underruns} predicted pump underruns in the game`);
        if (s.underruns > 0) fail(`${label}: ${s.underruns} underruns in the game, which fills audio from the frame loop`);
        checkAudibleWorklet(s, label);
        if (!s.running || s.rms < 0.01) fail(`${label}: music is silent: ${JSON.stringify(s)}`);
        console.log(`${label}: rate ${s.rate}, shared ${s.shared}, rms ${s.rms.toFixed(3)}, underruns ${s.underruns} (pump ${pump?.underruns})`);
        if (s.underruns > 0 || pump?.underruns > 0) {
            try {
                const timing = await js("JSON.stringify({now:performance.now(), timing:globalThis.__aegoAudioSmokeTiming ?? null})");
                console.log(`${label}: underrun timing (main-thread long tasks and timer gaps, ms since navigation; diagnostic only): ${timing}`);
            } catch {
                console.log(`${label}: underrun timing unavailable`);
            }
        }
        // The product mixer must keep feeding its worklet while a genuine
        // page-thread stall exceeds either transport's unchanged reserve.
        const stallMS = await js(`(() => {
            const start = performance.now();
            while (performance.now() - start < 500) {}
            return performance.now() - start;
        })()`);
        await sleep(300);
        const afterStall = await stats();
        const pumpAfterStall = await js("globalThis.aegoGame?.mixStats?.() ?? null");
        if (!Number.isFinite(stallMS) || stallMS < 500) fail(`${label}: the main-thread stall did not run`);
        if (afterStall.error) fail(`${label}: ${afterStall.error}`);
        if (!afterStall.running || afterStall.rms < 0.01) fail(`${label}: music stopped after the main-thread stall`);
        checkAudibleWorklet(afterStall, label);
        if (!Number.isSafeInteger(s.consumed) || !Number.isSafeInteger(afterStall.consumed) || afterStall.consumed <= s.consumed ||
            !Number.isSafeInteger(pump?.blocks) || !Number.isSafeInteger(pumpAfterStall?.blocks) || pumpAfterStall.blocks <= pump.blocks) {
            fail(`${label}: Worker output did not advance after the main-thread stall`);
        }
        if (!Number.isInteger(afterStall.underruns) || afterStall.underruns < 0 || afterStall.underruns > 0) {
            fail(`${label}: worklet underruns after the main-thread stall: ${afterStall.underruns}`);
        }
        if (!Number.isInteger(pumpAfterStall?.underruns) || pumpAfterStall.underruns < 0 || pumpAfterStall.underruns > 0) {
            fail(`${label}: predicted pump underruns after the main-thread stall: ${pumpAfterStall?.underruns}`);
        }
        console.log(`${label}: ${stallMS.toFixed(1)} ms injected main-thread stall, worklet underruns ${afterStall.underruns} (pump ${pumpAfterStall?.underruns})`);
        if (await page.isHidden("#mute")) fail(`${label}: the mute button is hidden`);
        await page.click("#mute");
        await sleep(300);
        const m = await stats();
        if (!m.muted) fail(`${label}: mute did not apply`);
        if (m.out !== 0) fail(`${label}: muted output is not silent: ${JSON.stringify(m)}`);
        await page.reload();
        await until("typeof globalThis.aegoAudioStats === 'function'", 120000);
        if (!(await stats()).muted) fail(`${label}: mute was not remembered`);
        await js("globalThis.aegoAudioMute(false)");
    } catch (e) {
        fail(`${label}: ${e.message}`);
    } finally {
        await page.close();
        srv.close();
    }
}

const browser = await chromium.launch({
    executablePath,
    headless: opts.headed !== "true",
    args: ["--mute-audio", ...(opts.gpu === "hard" ? [] : ["--use-angle=swiftshader", "--enable-unsafe-swiftshader"]), "--ignore-gpu-blocklist", "--autoplay-policy=user-gesture-required"],
});
try {
    await run(browser, "messages", {}, false);
    for (const b of blocks) await run(browser, `messages at ${b} blocks`, {}, false, b);
    await run(browser, "shared", isolate, true);
    await demo(browser, "demo", {}, false);
    await demo(browser, "demo shared", isolate, true);
} finally {
    await browser.close();
}
if (failures.length) {
    console.error(`${failures.length} failure(s)`);
    process.exit(1);
}
console.log("web audio: ok");
