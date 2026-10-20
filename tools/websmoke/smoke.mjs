import fs from "node:fs";
import http from "node:http";
import path from "node:path";
import { chromium } from "playwright-core";
import pixelmatch from "pixelmatch";
import pngjs from "pngjs";

const { PNG } = pngjs;
const opts = Object.fromEntries(
    process.argv.slice(2).map((a) => {
        const [k, v] = a.replace(/^--/, "").split("=");
        return [k, v ?? "true"];
    }),
);
const site = path.resolve(opts.site ?? "../../site");
const goldenDir = path.resolve(opts.golden ?? "golden");
const update = opts.update === "true";
const executablePath = process.env.CHROMIUM || "/usr/bin/chromium";
const mime = {
    ".html": "text/html",
    ".js": "text/javascript",
    ".css": "text/css",
    ".wasm": "application/wasm",
    ".json": "application/json",
    ".png": "image/png",
    ".svg": "image/svg+xml",
};
const failures = [];

function fail(msg) {
    failures.push(msg);
    console.error("FAIL " + msg);
}

function sleep(ms) {
    return new Promise((r) => setTimeout(r, ms));
}

function serve(root) {
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
            res.writeHead(200, { "Content-Type": mime[path.extname(file)] || "application/octet-stream" });
            fs.createReadStream(file).pipe(res);
        });
        srv.listen(0, "127.0.0.1", () => resolve(srv));
    });
}

async function waitFor(fn, what, timeout = 30000) {
    const end = Date.now() + timeout;
    for (;;) {
        const v = await fn();
        if (v) return v;
        if (Date.now() > end) throw new Error("timed out waiting for " + what);
        await sleep(100);
    }
}

const srv = await serve(site);
const base = `http://127.0.0.1:${srv.address().port}`;
const browser = await chromium.launch({
    executablePath,
    headless: true,
    args: ["--use-angle=swiftshader", "--enable-unsafe-swiftshader", "--ignore-gpu-blocklist"],
});
const page = await browser.newPage({ viewport: { width: 1280, height: 800 }, deviceScaleFactor: 1 });
page.on("pageerror", (e) => fail("page error: " + e.message));
page.on("console", (m) => {
    if (m.type() === "error") fail("console: " + m.text());
});

const call = (method, payload = {}) =>
    page.evaluate(([m, p]) => JSON.parse(globalThis.aego.call(m, JSON.stringify(p))), [method, payload]);
const stats = async () => (await call("poll")).stats;

async function compare(name) {
    await page.evaluate(() => ($("game-hint").hidden = true));
    const shot = PNG.sync.read(await page.locator("#game").screenshot());
    const file = path.join(goldenDir, name + ".png");
    if (update) {
        fs.mkdirSync(goldenDir, { recursive: true });
        fs.writeFileSync(file, PNG.sync.write(shot));
        console.log("wrote " + file);
        return;
    }
    if (!fs.existsSync(file)) {
        fail(`${name}: no golden, run make web-smoke-update`);
        return;
    }
    const want = PNG.sync.read(fs.readFileSync(file));
    if (want.width !== shot.width || want.height !== shot.height) {
        fail(`${name}: ${shot.width}x${shot.height}, golden ${want.width}x${want.height}`);
        return;
    }
    const diff = new PNG({ width: want.width, height: want.height });
    const n = pixelmatch(want.data, shot.data, diff.data, want.width, want.height, { threshold: 0.1 });
    const pct = (n * 100) / (want.width * want.height);
    if (pct > 0.5) {
        fs.writeFileSync(path.join(goldenDir, `actual-${name}.png`), PNG.sync.write(shot));
        fail(`${name}: ${pct.toFixed(2)}% of pixels differ`);
        return;
    }
    console.log(`${name}: ${pct.toFixed(3)}% of pixels differ`);
}

try {
    await page.goto(base + "/play/");
    const boot = await waitFor(() => page.evaluate(() => window.aegoBoot), "the playground", 120000);
    console.log(`cold start ${boot} ms`);

    const b64 = await call("bench", { count: 64, animate: false });
    if (!b64.started) throw new Error("bench did not start: " + b64.error);
    const s0 = await waitFor(async () => {
        const s = await stats();
        return s.frames >= 10 && s;
    }, "bench frames");
    console.log(`renderer ${s0.renderer}`);
    await compare("bench64");

    if (await call("loseContext", { lose: true })) {
        await waitFor(async () => (await stats()).lost, "the context loss");
        await call("loseContext", { lose: false });
        const before = (await stats()).frames;
        await waitFor(async () => {
            const s = await stats();
            return !s.lost && s.restores >= 1 && s.frames > before + 10;
        }, "the context restore");
        await compare("bench64");
        console.log("context loss: restored");
    } else {
        fail("WEBGL_lose_context is not available");
    }

    await call("bench", { count: 1000, animate: true });
    await sleep(1000);
    const a = await stats();
    await sleep(3000);
    const b = await stats();
    const report = {
        sprites: 1000,
        fps: Math.round(((b.frames - a.frames) / 3) * 10) / 10,
        frameMs: Math.round(b.frameMs * 100) / 100,
        glCalls: b.calls,
        drawCalls: b.drawCalls,
        renderer: b.renderer,
        coldStartMs: boot,
    };
    console.log("bench " + JSON.stringify(report));
    if (opts.report) fs.writeFileSync(opts.report, JSON.stringify(report, null, 2) + "\n");

    const examples = await call("examples");
    const scenes = examples.filter((e) => e.scene);
    if (!scenes.length) fail("there are no scene examples");
    for (const ex of scenes) {
        const res = await call("play", { example: ex.id, files: { "main.aes": ex.code } });
        const errors = (res.diagnostics || []).filter((d) => d.severity === "error");
        if (!res.started || errors.length) {
            fail(`${ex.id}: ${res.error || errors.map((d) => d.message).join("; ")}`);
            continue;
        }
        const start = (await stats()).frames;
        await sleep(2000);
        const p = await call("poll");
        const faults = p.faults || [];
        if (faults.length) fail(`${ex.id}: ${faults.map((f) => f.code + " " + f.message).join("; ")}`);
        else if (p.stats.frames < start + 30) fail(`${ex.id}: ${p.stats.frames - start} frames in 2 s`);
        else console.log(`${ex.id}: ok`);
    }
    await call("stop");
} catch (e) {
    fail(e.message);
} finally {
    await browser.close();
    srv.close();
}
if (failures.length) {
    console.error(`${failures.length} failure(s)`);
    process.exit(1);
}
console.log("web smoke: ok");
