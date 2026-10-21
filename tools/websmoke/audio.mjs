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

async function run(browser, label, headers, wantShared) {
    const srv = await serve(site, headers);
    const page = await browser.newPage({ viewport: { width: 1280, height: 800 }, deviceScaleFactor: 1 });
    page.on("pageerror", (e) => fail(`${label}: page error: ${e.message}`));
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
    const stats = async () => JSON.parse(await js("globalThis.aegoAudio.stats()"));
    try {
        await page.goto(`http://127.0.0.1:${srv.address().port}/play/`);
        await until("!!(globalThis.aegoAudio && globalThis.aegoAudio.start)", 120000);
        const isolated = await js("globalThis.crossOriginIsolated === true");
        if (isolated !== wantShared) fail(`${label}: crossOriginIsolated is ${isolated}`);
        const enforced = await js("(async () => { const c = new AudioContext(); await new Promise((r) => setTimeout(r, 300)); const s = c.state; await c.close(); return s !== 'running'; })()");
        const err = await js("globalThis.aegoAudio.start()");
        if (err) throw new Error("start: " + err);
        await sleep(500);
        const before = await stats();
        if (!enforced) console.log(`${label}: this browser starts an AudioContext without a gesture, the gesture check is skipped`);
        else if (before.running || before.queued !== 0) fail(`${label}: audio ran before a gesture: ${JSON.stringify(before)}`);
        await page.mouse.click(5, 5);
        await until("JSON.parse(globalThis.aegoAudio.stats()).running", 10000);
        await sleep(1500);
        const s = await stats();
        if (s.error) fail(`${label}: ${s.error}`);
        if (s.shared !== wantShared) fail(`${label}: shared transport is ${s.shared}`);
        if (Math.abs(s.rms - 0.354) > 0.05) fail(`${label}: rms ${s.rms}`);
        console.log(`${label}: rate ${s.rate}, shared ${s.shared}, rms ${s.rms.toFixed(3)}, underruns ${s.underruns}, mix max ${s.mixMaxUs} us`);
        await js("globalThis.aegoAudio.stop()");
    } catch (e) {
        fail(`${label}: ${e.message}`);
    } finally {
        await page.close();
        srv.close();
    }
}

async function demo(browser) {
    const label = "demo";
    if (!fs.existsSync(path.join(site, "demo", "index.html"))) {
        fail(`${label}: site/demo is missing`);
        return;
    }
    const srv = await serve(site, {});
    const page = await browser.newPage({ viewport: { width: 1280, height: 720 }, deviceScaleFactor: 1 });
    page.on("pageerror", (e) => fail(`${label}: page error: ${e.message}`));
    const stats = async () => JSON.parse(await page.evaluate(() => globalThis.aegoAudioStats()));
    const base = `http://127.0.0.1:${srv.address().port}/demo/`;
    try {
        await page.goto(base);
        await page.waitForFunction(() => globalThis.aegoBoot !== undefined && typeof globalThis.aegoAudioStats === "function", null, { timeout: 120000 });
        await page.evaluate(() => globalThis.aegoAudioMute(false));
        const before = await stats();
        if (before.running) fail(`${label}: audio ran before a gesture`);
        await page.click("#game");
        await sleep(2500);
        const s = await stats();
        if (!s.running || s.rms < 0.01) fail(`${label}: music is silent: ${JSON.stringify(s)}`);
        console.log(`${label}: rate ${s.rate}, shared ${s.shared}, rms ${s.rms.toFixed(3)}, underruns ${s.underruns}`);
        if (await page.isHidden("#mute")) fail(`${label}: the mute button is hidden`);
        await page.click("#mute");
        if (!(await stats()).muted) fail(`${label}: mute did not apply`);
        await page.reload();
        await page.waitForFunction(() => typeof globalThis.aegoAudioStats === "function", null, { timeout: 120000 });
        if (!(await stats()).muted) fail(`${label}: mute was not remembered`);
        await page.evaluate(() => globalThis.aegoAudioMute(false));
    } catch (e) {
        fail(`${label}: ${e.message}`);
    } finally {
        await page.close();
        srv.close();
    }
}

const browser = await chromium.launch({
    executablePath,
    headless: true,
    args: ["--use-angle=swiftshader", "--enable-unsafe-swiftshader", "--ignore-gpu-blocklist", "--autoplay-policy=user-gesture-required"],
});
try {
    await run(browser, "messages", {}, false);
    await run(browser, "shared", isolate, true);
    await demo(browser);
} finally {
    await browser.close();
}
if (failures.length) {
    console.error(`${failures.length} failure(s)`);
    process.exit(1);
}
console.log("web audio: ok");
