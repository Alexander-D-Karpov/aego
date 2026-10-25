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
const executablePath = process.env.CHROMIUM || "/usr/bin/chromium";
const mime = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".wasm": "application/wasm", ".json": "application/json" };
const goldenDir = path.resolve(opts.golden ?? "golden");
const update = opts.update === "true";
const failures = [];

function fail(msg) {
    failures.push(msg);
    console.error("FAIL " + msg);
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

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function startTry(url) {
    await page.goto(base + url);
    await page.waitForSelector(".try .try-stage");
    await page.evaluate(() => {
        const b = document.querySelector(".try .try-start");
        if (b) b.click();
    });
    await page.waitForFunction(
        () => {
            const s = document.querySelector(".try .try-status");
            if (s && s.classList.contains("error")) throw new Error(s.textContent);
            if (!globalThis.aegoDocs || !globalThis.aegoDocs.call) return false;
            const r = JSON.parse(globalThis.aegoDocs.call("state", "{}"));
            return r.state && r.state.frames >= 30;
        },
        null,
        { timeout: 120000, polling: 100 },
    );
}

const frameNow = async () => (await call("state")).state.frames;

async function golden(name, shot) {
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
    const n = pixelmatch(want.data, shot.data, null, want.width, want.height, { threshold: 0.1 });
    const pct = (n * 100) / (want.width * want.height);
    if (pct > 2) {
        fs.writeFileSync(path.join(goldenDir, `actual-${name}.png`), PNG.sync.write(shot));
        fail(`${name}: ${pct.toFixed(2)}% of pixels differ`);
        return;
    }
    console.log(`${name}: ${pct.toFixed(3)}% of pixels differ`);
}

// sound plays the Audio Source 2D try scene and expects the output to carry sound.
async function sound() {
    await startTry("/ref/audio.AudioSource2D.html");
    const box = await page.locator(".try canvas").boundingBox();
    await page.mouse.click(box.x + box.width * 0.2, box.y + box.height * 0.5);
    await sleep(1500);
    const st = JSON.parse(await page.evaluate(() => globalThis.aegoAudioStats()));
    if (!st.running || !(st.rms > 0)) fail(`try AudioSource2D is silent: ${JSON.stringify(st)}`);
    else console.log(`try: AudioSource2D plays, rms ${st.rms.toFixed(3)}`);
}

// motion holds ArrowRight for 60 frames in the Motion 2D try scene: the hint goes away on the click and the player moves right.
async function motion() {
    await startTry("/ref/gameplay.Motion2D.html");
    if (!(await page.locator(".try[data-input]").count())) fail("the Motion 2D try block does not ask for input");
    if (!(await page.locator(".try .try-input").count())) fail("the Motion 2D try scene shows no input hint");
    const from = (await call("pose", { node: "Player" })).pose;
    const box = await page.locator(".try canvas").boundingBox();
    await page.mouse.click(box.x + box.width * 0.2, box.y + box.height * 0.5);
    if (await page.locator(".try .try-input").count()) fail("the input hint stays after a click on the canvas");
    await page.keyboard.down("ArrowRight");
    const start = await frameNow();
    await page.waitForFunction((n) => JSON.parse(globalThis.aegoDocs.call("state", "{}")).state.frames >= n, start + 60, { timeout: 30000, polling: 16 });
    await page.keyboard.up("ArrowRight");
    const to = (await call("pose", { node: "Player" })).pose;
    if (!from || !to || !(to[0] > from[0] + 0.5)) fail(`the player did not move right: ${JSON.stringify(from)} -> ${JSON.stringify(to)}`);
    else console.log(`try: Motion2D moved from x ${from[0].toFixed(2)} to ${to[0].toFixed(2)}`);
    await call("play");
    await golden("try-motion2d", PNG.sync.read(await page.locator(".try canvas").screenshot()));
}

const srv = await serve(site);
const base = `http://127.0.0.1:${srv.address().port}`;
const browser = await chromium.launch({
    executablePath,
    headless: true,
    args: ["--mute-audio", "--use-angle=swiftshader", "--enable-unsafe-swiftshader", "--ignore-gpu-blocklist"],
});
const page = await browser.newPage({ viewport: { width: 1280, height: 900 }, deviceScaleFactor: 1, colorScheme: "dark" });
page.on("pageerror", (e) => fail("page error: " + e.message));
const call = (m, p = {}) => page.evaluate(([m, p]) => JSON.parse(globalThis.aegoDocs.call(m, JSON.stringify(p))), [m, p]);
const cell = '.try .st-fields .v[data-k$="component:physics.BoxCollider2D/restitution"]';
const restitution = () => page.locator(cell).first().textContent();

try {
    let docsFetches = 0;
    page.on("request", (r) => {
        if (r.url().includes("/try/aego-docs.wasm")) docsFetches++;
    });
    await page.goto(base + "/ref/physics.Body2D.html");
    const shared = await page.evaluate(async () => {
        const play = await globalThis.aegoRuntime("play", "aego.wasm", "aegoReady", "aego");
        const code = 'package demo\n\nimport "aego/log"\n\nscript S {\n\tfunc Ready() {\n\t\tlog.Info("shared")\n\t}\n}\n';
        return JSON.parse(play.call("run", JSON.stringify({ files: { "main.aes": code }, frames: 5 })));
    });
    if (shared.error || !(shared.log || []).some((l) => l.text.includes("shared"))) fail("the playground runtime did not run a snippet: " + JSON.stringify(shared));
    const started = Date.now();
    await page.click(".try .try-start");
    await page.waitForFunction(
        () => {
            const s = document.querySelector(".try .try-status");
            if (s && s.classList.contains("error")) throw new Error(s.textContent);
            if (!globalThis.aegoDocs || !globalThis.aegoDocs.call) return false;
            const r = JSON.parse(globalThis.aegoDocs.call("state", "{}"));
            return r.state && r.state.frames >= 30;
        },
        null,
        { timeout: 120000, polling: 100 },
    );
    console.log(`try: running after ${Date.now() - started} ms, next to the playground runtime`);
    const set = await call("set", { component: "physics.BoxCollider2D", field: "restitution", value: 0.75 });
    if (set.error) throw new Error("set: " + set.error);
    await page.waitForFunction((sel) => {
        const c = document.querySelector(sel);
        return c && c.textContent === "0.75" && c.classList.contains("changed");
    }, cell, { timeout: 10000 });
    const st = (await call("state")).state;
    if (st.restarts < 1 || !st.running) fail(`the edit did not restart the scene: ${JSON.stringify(st)}`);
    await page.click(".try .try-reset");
    await page.waitForFunction((sel) => !document.querySelector(sel).classList.contains("changed"), cell, { timeout: 10000 }).catch(() => {});
    if ((await restitution()) === "0.75") fail("Reset kept the edit");
    const again = await page.evaluate(async () => {
        const a = await globalThis.aegoRuntime("try", "aego-docs.wasm", "aegoDocsReady", "aegoDocs");
        const b = await globalThis.aegoRuntime("play", "aego.wasm", "aegoReady", "aego");
        return a === globalThis.aegoDocs && JSON.parse(b.call("run", JSON.stringify({ files: { "main.aes": "package demo\n" }, frames: 1 }))).frames >= 1;
    });
    if (!again) fail("a second request did not reuse the runtimes");
    if (docsFetches !== 1) fail(`aego-docs.wasm was fetched ${docsFetches} times`);
    else console.log("try: edit, restart, reset and runtime reuse work");
    await sound();
    await motion();
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
console.log("web try: ok");
