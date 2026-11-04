import fs from "node:fs";
import http from "node:http";
import path from "node:path";
import { chromium } from "playwright-core";
import pixelmatch from "pixelmatch";
import pngjs from "pngjs";
import { frameLimitOutcome } from "./ci-warning-policy.mjs";

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
    ".wasm": "application/wasm",
    ".json": "application/json",
    ".png": "image/png",
    ".aetext": "text/plain",
};
const failures = [];
const warnings = [];

function fail(msg) {
    failures.push(msg);
    console.error("FAIL " + msg);
}

function warn(msg) {
    warnings.push(msg);
    console.warn("WARN " + msg);
    if (process.env.GITHUB_ACTIONS) console.warn("::warning title=SwiftShader frame limit::" + msg);
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

async function frames(page, n, timeout) {
    await page.waitForFunction(
        (want) => {
            const err = document.getElementById("error");
            if (err && !err.hidden) throw new Error(err.textContent);
            return globalThis.aegoGame && globalThis.aegoGame.frames() >= want;
        },
        n,
        { timeout, polling: 100 },
    );
}

async function compare(page, name) {
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
    const n = pixelmatch(want.data, shot.data, null, want.width, want.height, { threshold: 0.1 });
    const pct = (n * 100) / (want.width * want.height);
    if (pct > 2) {
        fs.writeFileSync(path.join(goldenDir, `actual-${name}.png`), PNG.sync.write(shot));
        fail(`${name}: ${pct.toFixed(2)}% of pixels differ`);
        return;
    }
    console.log(`${name}: ${pct.toFixed(3)}% of pixels differ`);
}


// touchPad drives the demo on a phone through the touch gamepad and checks the game reads its stick.
async function touchPad() {
    const ctx = await browser.newContext({ viewport: { width: 844, height: 390 }, deviceScaleFactor: 1, hasTouch: true, isMobile: true });
    const phone = await ctx.newPage();
    phone.on("pageerror", (e) => fail("phone page error: " + e.message));
    await phone.goto(base + "/demo/");
    await frames(phone, 120, 120000);
    if (!(await phone.locator("#vpad .stick").isVisible())) {
        fail("the touch gamepad is hidden on a phone");
        return;
    }
    const box = await phone.locator("#vpad .stick").boundingBox();
    const cdp = await ctx.newCDPSession(phone);
    const at = (x) => [{ x: box.x + box.width / 2 + x, y: box.y + box.height / 2, id: 1 }];
    const leftX = async () => {
        const pads = await phone.evaluate(() => globalThis.aegoGame.pads());
        const touch = pads.find((p) => p.name === "Touch Gamepad");
        return touch ? touch.leftX : null;
    };
    await cdp.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: at(0) });
    await cdp.send("Input.dispatchTouchEvent", { type: "touchMove", touchPoints: at(box.width / 2) });
    await phone.waitForTimeout(200);
    const held = await leftX();
    await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
    // The virtual pad resets on pointerup, but the game reads it on its next frame.
    const releaseFrame = await phone.evaluate(() => globalThis.aegoGame.frames());
    await frames(phone, releaseFrame + 1, 120000);
    const released = await leftX();
    if (held === null || held < 0.9 || released !== 0) fail(`the game reads the touch stick as ${held}, then ${released} after release`);
    else console.log("demo: the game reads the touch gamepad");
    await menuAndFullscreen(phone, cdp);
    await ctx.close();
}

// menuAndFullscreen checks that the gamepad gives way to the settings screen and stays on screen in fullscreen.
async function menuAndFullscreen(phone, cdp) {
    const tap = async (sel) => {
        const b = await phone.locator(sel).boundingBox();
        const pt = [{ x: b.x + b.width / 2, y: b.y + b.height / 2, id: 2 }];
        await cdp.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: pt });
        await phone.waitForTimeout(150);
        await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
    };
    const padShown = () => phone.locator("#vpad .stick").isVisible();
    await tap("#vpad .start");
    await phone.waitForTimeout(600);
    const open = await phone.evaluate(() => globalThis.aegoGame.menuOpen());
    if (!open) fail("Start on the touch gamepad does not open the settings screen");
    else if (await padShown()) fail("the touch gamepad covers the open settings screen");
    else console.log("demo: the touch gamepad hides under the settings screen");
    if (opts.shots) await phone.screenshot({ path: path.join(opts.shots, "phone-settings.png") });
    await phone.keyboard.press("Escape");
    await phone.waitForTimeout(600);
    if (await phone.evaluate(() => globalThis.aegoGame.menuOpen())) fail("Escape does not close the settings screen");
    else if (!(await padShown())) fail("the touch gamepad does not come back after the settings screen closes");
    await phone.locator("#fullscreen").click();
    await phone.waitForTimeout(500);
    const full = await phone.evaluate(() => document.fullscreenElement === document.documentElement);
    if (!full) fail("the fullscreen button does not make the page fullscreen");
    else if (!(await padShown())) fail("the touch gamepad is hidden in fullscreen");
    else console.log("demo: the touch gamepad stays in fullscreen");
    if (opts.shots) await phone.screenshot({ path: path.join(opts.shots, "phone-fullscreen.png") });
}

const srv = await serve(site);
async function frameLimit() {
    await page.evaluate(() => globalThis.aegoGame.frameLimit(30));
    await page.waitForTimeout(500);
    const f0 = await page.evaluate(() => globalThis.aegoGame.frames());
    await page.waitForTimeout(3000);
    const f1 = await page.evaluate(() => globalThis.aegoGame.frames());
    await page.evaluate(() => globalThis.aegoGame.frameLimit(0));
    const fps = (f1 - f0) / 3;
    let renderer = "";
    if (fps < 26) {
        try { renderer = await page.evaluate(() => JSON.parse(globalThis.aegoGame.stats()).renderer || ""); }
        catch { /* An unverified renderer must not qualify for a warning. */ }
    }
    const outcome = frameLimitOutcome(fps, renderer, process.env.AEGO_CI_WEB_WARN === "1");
    if (outcome === "warning") warn(`a 30 fps limit gives ${fps.toFixed(1)} fps on SwiftShader (required 26–32; Stage 13.5)`);
    else if (outcome === "fail") fail(`a 30 fps limit gives ${fps.toFixed(1)} fps`);
    else console.log(`demo: frame limit 30 gives ${fps.toFixed(1)} fps`);
}

async function pixelRatio() {
    const ctx = await browser.newContext({ viewport: { width: 1280, height: 720 }, deviceScaleFactor: 2 });
    const hi = await ctx.newPage();
    hi.on("pageerror", (e) => fail("high-density page error: " + e.message));
    await hi.goto(base + "/demo/");
    await frames(hi, 30, 120000);
    const r = await hi.evaluate(() => {
        const c = document.getElementById("game");
        return c.width / c.getBoundingClientRect().width;
    });
    if (Math.abs(r - 1.5) > 0.01) fail(`at device pixel ratio 2 the canvas renders at ${r.toFixed(3)}, want the 1.5 limit`);
    else console.log("demo: the pixel ratio is capped at 1.5");
    await ctx.close();
}

const base = `http://127.0.0.1:${srv.address().port}`;
const browser = await chromium.launch({
    executablePath,
    headless: true,
    args: ["--mute-audio", "--use-angle=swiftshader", "--enable-unsafe-swiftshader", "--ignore-gpu-blocklist"],
});
const page = await browser.newPage({ viewport: { width: 1280, height: 720 }, deviceScaleFactor: 1 });
page.on("pageerror", (e) => fail("page error: " + e.message));

try {
    await page.goto(base + "/demo/");
    await frames(page, 120, 120000);
    console.log(`demo: cold start ${await page.evaluate(() => window.aegoBoot)} ms`);
    await compare(page, "demo");
    const err = await page.evaluate(() => globalThis.aegoGame.userWrite("smoke/check.txt", "kept"));
    if (err) throw new Error("userWrite: " + err);
    await page.reload();
    await frames(page, 10, 120000);
    const kept = await page.evaluate(() => globalThis.aegoGame.userRead("smoke/check.txt"));
    if (kept !== "kept") fail(`the profile did not survive a reload: ${kept}`);
    else console.log("demo: profile survives a reload");
    await touchPad();
    await frameLimit();
    await pixelRatio();
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
if (warnings.length) console.log(`web demo: completed with ${warnings.length} warning(s); FPS check did not pass`);
else console.log("web demo: ok");
