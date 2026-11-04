// Real exported Materials demo: static files only, sequential browser contexts.
// Captures are inspected in memory; the report contains measurements, not images.
import fs from "node:fs";
import http from "node:http";
import path from "node:path";
import crypto from "node:crypto";
import pngjs from "pngjs";
const { chromium, firefox } = await import(process.env.PLAYWRIGHT_MODULE ?? "playwright-core");
const { PNG } = pngjs;
const opts = Object.fromEntries(process.argv.slice(2).map(arg => {
    const at = arg.indexOf("=");
    return at < 0 ? [arg.replace(/^--/, ""), "true"] : [arg.slice(2, at), arg.slice(at + 1)];
}));
const root = path.resolve(opts.root ?? "../../.cache/export-materials-web");
const out = path.resolve(opts.report ?? "../../.cache/export-materials-web.json");
const mime = { ".html": "text/html", ".js": "text/javascript", ".wasm": "application/wasm", ".json": "application/json" };
const hash = bytes => crypto.createHash("sha256").update(bytes).digest("hex");
const files = JSON.parse(fs.readFileSync(path.join(root, "data/files.json"), "utf8"));
const manifest = fs.readFileSync(path.join(root, "export.aetext"), "utf8");
const report = {
    createdUTC: new Date().toISOString(), root, exportHash: manifest.match(/\bhash="([a-f0-9]+)"/)?.[1],
    wasmSHA256: hash(fs.readFileSync(path.join(root, "game.wasm"))),
    wasmBytes: files.wasm, wasmGzipBytes: files.wasmGzip, artifacts: files.files.length,
    serving: "static HTTP export root; no importer or development server", viewport: [1280, 720], DPR: 1,
    browsers: [],
};
const server = http.createServer((req, res) => {
    let name;
    try { name = decodeURIComponent(new URL(req.url, "http://localhost").pathname); }
    catch { res.writeHead(400); res.end(); return; }
    if (name === "/favicon.ico") { res.writeHead(204); res.end(); return; }
    if (name.endsWith("/")) name += "index.html";
    const file = path.resolve(root, name.replace(/^\/+/, ""));
    if (!file.startsWith(root + path.sep) || !fs.existsSync(file) || !fs.statSync(file).isFile()) {
        res.writeHead(404); res.end(); return;
    }
    res.writeHead(200, { "Content-Type": mime[path.extname(file)] ?? "application/octet-stream" });
    fs.createReadStream(file).pipe(res);
});
await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));
const base = `http://127.0.0.1:${server.address().port}`;

async function stats(page) { return page.evaluate(() => JSON.parse(globalThis.aegoGame.stats())); }
async function frames(page, target) {
    await page.waitForFunction(want => {
        const box = document.getElementById("error");
        if (box && !box.hidden) throw new Error(box.textContent);
        return globalThis.aegoGame && globalThis.aegoGame.frames() >= want;
    }, target, { timeout: 120000, polling: 100 });
}
async function capture(page) {
    const image = PNG.sync.read(await page.locator("#game").screenshot());
    const frame = await page.evaluate(() => globalThis.aegoGame.frames());
    return { image, frame, sha256: hash(image.data) };
}
function changed(a, b, rect) {
    if (a.width !== b.width || a.height !== b.height) throw new Error("canvas dimensions changed unexpectedly");
    const [x, y, w, h] = rect;
    let count = 0;
    for (let py = y; py < y + h; py++) for (let px = x; px < x + w; px++) {
        const i = (py * a.width + px) * 4;
        if (Math.abs(a.data[i] - b.data[i]) + Math.abs(a.data[i + 1] - b.data[i + 1]) + Math.abs(a.data[i + 2] - b.data[i + 2]) > 6) count++;
    }
    return count;
}
function imageInfo(image) {
    const colors = new Set();
    for (let i = 0; i < image.data.length; i += 64) {
        colors.add(image.data.readUInt32LE(i));
        if (colors.size > 256) break;
    }
    return { width: image.width, height: image.height, sampledColors: colors.size };
}
async function animation(page, result, label) {
    const shots = [await capture(page)];
    for (let i = 0; i < 2; i++) {
        await frames(page, shots.at(-1).frame + 20);
        shots.push(await capture(page));
    }
    // These are non-overlapped portions of the two animated sprites in the
    // documented 1280x720 demo. The static third A sprite is outside both.
    const regions = { flash: [150, 315, 100, 100], dissolve: [395, 325, 34, 110] };
    const changes = Object.fromEntries(Object.entries(regions).map(([name, rect]) => [name,
        Math.max(...shots.slice(1).map(s => changed(shots[0].image, s.image, rect)))]));
    result[label] = { frames: shots.map(s => s.frame), hashes: shots.map(s => s.sha256), changes, ...imageInfo(shots[0].image) };
    if (result[label].sampledColors < 16) throw new Error(`${label}: rendered canvas is empty or uniform`);
    for (const [name, count] of Object.entries(changes)) if (count < 10) throw new Error(`${label}: ${name} sprite did not visibly animate (${count} changed pixels)`);
    result.passed.push(`${label}: Flash and Dissolve visibly animate`);
}
function traffic(a, b) {
    const n = b.frames - a.frames;
    if (n <= 0) throw new Error("no rendered frames in measured window");
    const bridge = {};
    for (const [k, value] of Object.entries(b.bridge ?? {})) bridge[k] = (value - (a.bridge?.[k] ?? 0)) / n;
    return { frames: n, drawsPerFrame: (b.draws - a.draws) / n, spritesPerFrame: (b.sprites - a.sprites) / n, bridgePerFrame: bridge };
}

try {
    for (const name of (opts.browsers ?? "chromium,firefox").split(",")) {
        if (!["chromium", "firefox"].includes(name)) throw new Error(`unknown browser ${name}`);
        const isFirefox = name === "firefox";
        const result = { browser: name, passed: [], failures: [], console: [], requests: [] };
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
            const page = await browser.newPage({ viewport: { width: 1280, height: 720 }, deviceScaleFactor: 1 });
            page.on("pageerror", e => result.failures.push(`page error: ${e.message}`));
            page.on("console", message => {
                result.console.push({ type: message.type(), text: message.text() });
                if (message.type() === "error" || /\bERROR\b|panic:|WebGL:.*INVALID_/.test(message.text())) result.failures.push(`console: ${message.text()}`);
            });
            page.on("requestfailed", req => result.failures.push(`request failed: ${req.url()} ${req.failure()?.errorText}`));
            page.on("response", res => {
                result.requests.push({ path: new URL(res.url()).pathname, status: res.status() });
                if (res.status() >= 400) result.failures.push(`HTTP ${res.status()}: ${res.url()}`);
            });
            await page.goto(base + "/?seed=12");
            await frames(page, 60);
            result.bootMs = await page.evaluate(() => globalThis.aegoBoot);
            const start = await stats(page);
            result.renderer = start.renderer;
            result.passed.push("export boots and advances 60 real frames");
            await animation(page, result, "beforeLoss");
            result.traffic = traffic(start, await stats(page));
            await page.evaluate(() => {
                const c = document.getElementById("game");
                globalThis.__exportEvents = { lost: 0, restored: 0 };
                c.addEventListener("webglcontextlost", () => globalThis.__exportEvents.lost++);
                c.addEventListener("webglcontextrestored", () => globalThis.__exportEvents.restored++);
            });
            if (!await page.evaluate(() => globalThis.aegoGame.loseContext(true))) throw new Error("WEBGL_lose_context unavailable");
            await page.waitForFunction(() => globalThis.__exportEvents.lost === 1 && JSON.parse(globalThis.aegoGame.stats()).lost, null, { timeout: 30000 });
            result.lostFrame = (await stats(page)).frames;
            if (!await page.evaluate(() => globalThis.aegoGame.loseContext(false))) throw new Error("restore request rejected");
            await page.waitForFunction(() => {
                const s = JSON.parse(globalThis.aegoGame.stats());
                return globalThis.__exportEvents.restored === 1 && !s.lost && s.restores >= 1;
            }, null, { timeout: 30000 });
            const restored = await stats(page);
            await frames(page, restored.frames + 30);
            result.contextEvents = await page.evaluate(() => globalThis.__exportEvents);
            result.restoredFrame = restored.frames;
            result.passed.push("real context loss and restore events; frames continue");
            await animation(page, result, "afterRestore");
            result.finalStats = await stats(page);
            if (result.requests.some(r => /\.(?:aeshader|aemat|go)$/.test(r.path))) throw new Error("runtime requested authoring source");
            result.passed.push("all runtime requests served from prepared export files");
        } catch (error) {
            result.failures.push(error.stack ?? String(error));
        } finally {
            if (browser) await browser.close();
        }
        console.log(`${name} ${result.version ?? "unavailable"}: ${result.passed.length} checks passed, ${result.failures.length} failed`);
        for (const failure of result.failures) console.error(failure);
    }
} finally {
    await new Promise(resolve => server.close(resolve));
    fs.mkdirSync(path.dirname(out), { recursive: true });
    fs.writeFileSync(out, JSON.stringify(report, null, 2) + "\n");
}
if (report.browsers.some(r => r.failures.length)) process.exitCode = 1;
