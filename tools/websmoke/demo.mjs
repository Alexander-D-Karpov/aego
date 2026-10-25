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
    ".wasm": "application/wasm",
    ".json": "application/json",
    ".png": "image/png",
    ".aetext": "text/plain",
};
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
        await phone.waitForTimeout(200);
        const pads = await phone.evaluate(() => globalThis.aegoGame.pads());
        const touch = pads.find((p) => p.name === "Touch Gamepad");
        return touch ? touch.leftX : null;
    };
    await cdp.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: at(0) });
    await cdp.send("Input.dispatchTouchEvent", { type: "touchMove", touchPoints: at(box.width / 2) });
    const held = await leftX();
    await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
    const released = await leftX();
    if (held === null || held < 0.9 || released !== 0) fail(`the game reads the touch stick as ${held}, then ${released} after release`);
    else console.log("demo: the game reads the touch gamepad");
    await ctx.close();
}

const srv = await serve(site);
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
console.log("web demo: ok");
