import fs from "node:fs";
import http from "node:http";
import path from "node:path";
import { chromium } from "playwright-core";
import pixelmatch from "pixelmatch";
import pngjs from "pngjs";
import { installTrafficProbe, summarizeTraffic, measureTrafficFrames } from "./traffic.mjs";
import { pauseAtMaterialFlash } from "./try-pulse.mjs";

const { PNG } = pngjs;

const opts = Object.fromEntries(
    process.argv.slice(2).map((a) => {
        const [k, v] = a.replace(/^--/, "").split("=");
        return [k, v ?? "true"];
    }),
);
const measureOnly = opts["measure-only"] === "true";
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

async function clickTryCanvas() {
    const canvas = page.locator(".try canvas");
    // Guide examples can be below the viewport. Pointer coordinates from an
    // unscrolled bounding box do not represent a click on the canvas.
    await canvas.scrollIntoViewIfNeeded();
    const box = await canvas.boundingBox();
    if (!box) throw new Error("Try canvas has no clickable bounds");
    await page.mouse.click(box.x + box.width * 0.2, box.y + box.height * 0.5);
}

async function clickTryInspector() {
    const canvas = page.locator(".try canvas");
    await canvas.scrollIntoViewIfNeeded();
    const box = await canvas.boundingBox();
    const viewport = (await call("viewport")).viewport;
    if (!box || !viewport || viewport.x + viewport.width + 20 >= viewport.canvasWidth || viewport.y + 10 >= viewport.canvasHeight)
        throw new Error("Try Inspector is not beside the Game View");
    await canvas.click({ position: {
        x: (viewport.x + viewport.width + 20) * box.width / viewport.canvasWidth,
        y: (viewport.y + 10) * box.height / viewport.canvasHeight,
    } });
}

async function runningAfterRestart(previous = -1, frames = 30) {
    await page.waitForFunction(({ previous, frames }) => {
        const status = document.querySelector(".try .try-status");
        if (status?.classList.contains("error")) throw new Error(status.textContent);
        const state = JSON.parse(globalThis.aegoDocs.call("state", "{}")).state;
        return state?.running && !state.paused && state.frames >= frames && state.restarts > previous;
    }, { previous, frames }, { timeout: 120000, polling: 100 });
}

async function inspect(component, expected) {
    const result = await call("inspect", { component, fields: Object.keys(expected) });
    if (result.error || !result.inspection?.runtimeExecuting) throw new Error(`${component} inspection: ${JSON.stringify(result)}`);
    for (const [name, want] of Object.entries(expected)) {
        if (result.inspection.observables?.[name] !== want)
            throw new Error(`${name}: expected ${JSON.stringify(want)}, got ${JSON.stringify(result.inspection.observables?.[name])}`);
    }
}

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

// The component demo starts a deliberately silent voice; the original guide
// still checks nonzero output through Chromium's muted virtual audio sink.
async function sound() {
    await startTry("/ref/audio.AudioSource2D.html");
    await clickTryCanvas();
    const component = "audio.AudioSource2D";
    const fields = (playing) => ({ [`${component}.PlayOnStart`]: playing, [`${component}.Playing`]: playing });
    await inspect(component, fields(false));
    const previous = (await call("state")).state.restarts;
    const edited = await call("set", { component, field: "PlayOnStart", value: 1 });
    if (edited.error) throw new Error("audio source edit: " + edited.error);
    await runningAfterRestart(previous);
    await inspect(component, fields(true));
    const reset = await call("reset");
    if (reset.error) throw new Error("audio source reset: " + reset.error);
    await runningAfterRestart();
    await inspect(component, fields(false));
    console.log("try: AudioSource2D starts and resets its real silent voice");

    await startTry("/audio.html");
    await clickTryCanvas();
    await sleep(1500);
    const st = JSON.parse(await page.evaluate(() => globalThis.aegoAudioStats()));
    if (!st.running || !(st.rms > 0)) fail(`audio guide is silent: ${JSON.stringify(st)}`);
    else console.log(`try: original audio guide plays through muted browser, rms ${st.rms.toFixed(3)}`);
}

// Check the focused component's actual body, then retain the original guide's
// keyboard, input hint, Reset and golden-image checks.
async function motion() {
    await startTry("/ref/gameplay.Motion2D.html");
    const origin = (await call("pose", { node: "Motion marker" })).pose;
    if (!origin || origin.length !== 2) throw new Error("the Motion 2D component marker is missing");
    const previous = (await call("state")).state.restarts;
    const edited = await call("set", { component: "gameplay.Motion2D", field: "MaxSpeed", value: 6 });
    if (edited.error) throw new Error("motion component edit: " + edited.error);
    await runningAfterRestart(previous);
    const moved = (await call("pose", { node: "Motion marker" })).pose;
    if (!moved || moved[0] <= origin[0] + 0.5) throw new Error(`Motion 2D component did not move: ${JSON.stringify(origin)} -> ${JSON.stringify(moved)}`);
    const componentReset = await call("reset");
    if (componentReset.error) throw new Error("motion component reset: " + componentReset.error);
    await runningAfterRestart();
    const restored = (await call("pose", { node: "Motion marker" })).pose;
    if (!restored || restored.some((v, i) => Math.abs(v - origin[i]) > 1e-6))
        throw new Error(`Motion 2D component did not reset: ${JSON.stringify(origin)} -> ${JSON.stringify(restored)}`);
    console.log("try: Motion2D component moved its body and reset");

    await startTry("/gameplay/motion.html");
    if (!(await page.locator(".try[data-input]").count())) fail("the Motion 2D try block does not ask for input");
    if (!(await page.locator(".try .try-input").count())) fail("the Motion 2D try scene shows no input hint");
    const from = (await call("pose", { node: "Player" })).pose;
    // The Inspector and game share one canvas. Clicking the Inspector focuses
    // that canvas but must not dismiss the hint or route keys into the game.
    await clickTryInspector();
    if (!(await page.locator(".try .try-input").count())) fail("clicking the Inspector hid the game input hint");
    await page.keyboard.down("ArrowRight");
    const beforeInspectorKey = await frameNow();
    await page.waitForFunction((n) => JSON.parse(globalThis.aegoDocs.call("state", "{}")).state.frames >= n, beforeInspectorKey + 20, { timeout: 30000, polling: 16 });
    await page.keyboard.up("ArrowRight");
    const beforeGameFocus = (await call("pose", { node: "Player" })).pose;
    if (!from || !beforeGameFocus || Math.abs(beforeGameFocus[0] - from[0]) > 0.1)
        fail(`the Inspector click routed keyboard input into the game: ${JSON.stringify(from)} -> ${JSON.stringify(beforeGameFocus)}`);
    await clickTryCanvas();
    if (await page.locator(".try .try-input").count()) fail("the input hint stays after a click on the canvas");
    await page.keyboard.down("ArrowRight");
    const start = await frameNow();
    await page.waitForFunction((n) => JSON.parse(globalThis.aegoDocs.call("state", "{}")).state.frames >= n, start + 60, { timeout: 30000, polling: 16 });
    await page.keyboard.up("ArrowRight");
    const to = (await call("pose", { node: "Player" })).pose;
    if (!from || !to || !(to[0] > from[0] + 0.5)) fail(`the player did not move right: ${JSON.stringify(from)} -> ${JSON.stringify(to)}`);
    else console.log(`try: Motion2D moved from x ${from[0].toFixed(2)} to ${to[0].toFixed(2)}`);
    // Assert input on the live world, then reset and pause for the UI image so timing-dependent physics and camera follow cannot change its pose.
    const reset = await page.evaluate(() => {
        const invoke = (m, p = {}) => JSON.parse(globalThis.aegoDocs.call(m, JSON.stringify(p)));
        const loaded = invoke("reset");
        if (loaded.error) throw new Error("motion reset: " + loaded.error);
        const paused = invoke("play");
        if (paused.error) throw new Error("motion pause: " + paused.error);
        return { state: paused.state, pose: invoke("pose", { node: "Player" }).pose };
    });
    if (!reset.state?.paused || reset.state.frames !== 0 || reset.pose?.length !== 2 || reset.pose.some(v => Math.abs(v) > 1e-6)) {
        throw new Error("the motion image did not reset to its paused initial state: " + JSON.stringify(reset));
    }
    await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
    const still = (await call("state")).state;
    if (!still.paused || still.frames !== 0) throw new Error("the reset motion scene advanced before its image was captured");
    const canvas = page.locator(".try canvas");
    const geometry = await canvas.evaluate(c => {
        const r = c.getBoundingClientRect();
        return { x: r.x, y: r.y, width: r.width, height: r.height, drawingWidth: c.width, drawingHeight: c.height, DPR: devicePixelRatio };
    });
    console.log("try: paused initial image " + JSON.stringify({ pose: reset.pose, frames: still.frames, geometry }));
    const translate = await canvas.evaluate(c => {
        const old = c.style.translate;
        const r = c.getBoundingClientRect(), d = devicePixelRatio;
        c.style.translate = `${Math.round(r.x * d) / d - r.x}px ${Math.round(r.y * d) / d - r.y}px`;
        return old;
    });
    let shot;
    try {
        shot = await canvas.screenshot();
    } finally {
        await canvas.evaluate((c, old) => { c.style.translate = old; }, translate);
    }
    if (opts.shot) fs.writeFileSync(opts.shot, shot);
    await golden("try-motion2d", PNG.sync.read(shot));
}

// A real pointerdown and a key tap in the same browser task must reach the
// freshly focused game. Holding B or injecting input after several frames
// would miss the first-frame routing regression this checks.
async function focusAndTap() {
    await startTry("/animation/player.html");
    const component = "animation.AnimationPlayer", field = `${component}.Speed`;
    const speed = async () => {
        const reply = await call("inspect", { component, fields: [field] });
        if (reply.error || !reply.inspection?.runtimeExecuting || typeof reply.inspection.observables?.[field] !== "number")
            throw new Error(`try Animation Player speed is unavailable: ${JSON.stringify(reply)}`);
        return reply.inspection.observables[field];
    };
    const before = await speed();
    if (before === 0) throw new Error("try Animation Player started with zero speed");
    const canvas = page.locator(".try canvas");
    await canvas.scrollIntoViewIfNeeded();
    const box = await canvas.boundingBox();
    const viewport = (await call("viewport")).viewport;
    if (!box || !viewport || viewport.width <= 50 || viewport.height <= 30)
        throw new Error("try Animation Player Game View is unavailable");
    await canvas.evaluate((element) => {
        globalThis.__aegoFocusProbe = null;
        element.addEventListener("pointerdown", (event) => {
            globalThis.__aegoFocusProbe = { trusted: event.isTrusted, primary: event.isPrimary, button: event.button };
            element.dispatchEvent(new KeyboardEvent("keydown", { bubbles: true, cancelable: true, code: "KeyB", key: "b" }));
            element.dispatchEvent(new KeyboardEvent("keyup", { bubbles: true, cancelable: true, code: "KeyB", key: "b" }));
        }, { once: true });
    });
    await canvas.click({ position: {
        x: (viewport.x + 50) * box.width / viewport.canvasWidth,
        y: (viewport.y + 30) * box.height / viewport.canvasHeight,
    } });
    const pointer = await page.evaluate(() => globalThis.__aegoFocusProbe);
    if (!pointer?.trusted || !pointer.primary || pointer.button !== 0)
        throw new Error(`try Animation Player pointer was not a trusted primary click: ${JSON.stringify(pointer)}`);
    await page.waitForFunction(({ component, field, expected }) => {
        const reply = JSON.parse(globalThis.aegoDocs.call("inspect", JSON.stringify({ component, fields: [field] })));
        return reply.inspection?.runtimeExecuting && reply.inspection.observables?.[field] === expected;
    }, { component, field, expected: -before }, { timeout: 15000, polling: 50 });
    console.log(`try: Animation Player first focused tap reversed speed ${before} -> ${await speed()}`);
}

// Exercise the published guide's albedo/normal/material bundle and moving-light script; static HTML cannot prove the docs WASM runtime can mount and play it.
async function normalMapping(url = "/render2d/normal-mapping.html") {
    await startTry(url);
    const bundle = await page.locator(".try").getAttribute("data-artifacts");
    if (!bundle || Buffer.from(bundle, "base64").subarray(0, 4).toString() !== "AETB")
        throw new Error("the normal-mapping guide did not supply its prepared asset bundle");
    const from = (await call("pose", { node: "Moving light" })).pose;
    if (!from || from.length !== 2) throw new Error("the guide's moving light is missing");
    await page.waitForFunction(([x, y]) => {
        const r = JSON.parse(globalThis.aegoDocs.call("pose", JSON.stringify({ node: "Moving light" })));
        return r.pose && Math.hypot(r.pose[0] - x, r.pose[1] - y) > 0.25;
    }, from, { timeout: 15000 });
    const edit = await call("set", { component: "engine.Light2D", field: "height", value: 3.25 });
    if (edit.error) throw new Error("edit light height: " + edit.error);
    // The document publishes its new text and the restarted world on Tick.
    await page.waitForFunction(() => {
        const c = document.querySelector('.try .st-fields .v[data-k$="component:engine.Light2D/height"]');
        return c?.textContent === "3.25" && c.classList.contains("changed");
    }, null, { timeout: 15000 });
    console.log("try: prepared normal-mapping example mounted, light moved, height edit applied");
}

// Exercise edits, restarts and the scripted material pulse in both language versions, not just the presence of a Try it block.
async function lightingAndMaterials() {
    const cases = [
        { name: "Light2D", node: "Warm point light", field: "height", value: 3.25, text: "3.25" },
        { name: "LightOccluder2D", node: "Manual occluder", field: "closed", value: 0, text: "false" },
        { name: "LightingEnvironment2D", node: "Lighting environment", field: "ambientIntensity", value: 0.37, text: "0.37" },
        { name: "MaterialOverride", node: "Flash instance", animated: true },
        { name: "Mesh2D", node: "Mesh 2D", field: "order", value: 0, text: "0", initial: "2", visualOrder: true },
        { name: "SpriteRenderer", node: "Sprite", field: "order", value: -1, text: "-1", initial: "1", visualOrder: true },
    ];
    for (const prefix of ["", "/ru"]) {
        for (const example of cases) {
            if (opts["mesh-only"] === "true" && example.name !== "Mesh2D") continue;
            if (opts["components-only"] === "true" && !example.visualOrder) continue;
            const component = `engine.${example.name}`;
            await startTry(`${prefix}/ref/${component}.html`);
            if (await page.locator(".try").first().getAttribute("data-select") !== component)
                throw new Error(`try ${component}: incorrect component selection`);
            const pose = await call("pose", { node: example.node });
            if (!pose.pose || pose.pose.length !== 2) throw new Error(`try ${component}: missing ${example.node}`);
            if (example.animated) {
                const fields = (value, explicit) => ({
                    "engine.MaterialOverride.FlashValue": value,
                    "engine.MaterialOverride.FlashExplicit": explicit,
                    "engine.MaterialOverride.ReferenceFlash": 0,
                });
                await inspect(component, fields(0, false));
                const previous = (await call("state")).state.restarts;
                const toggle = await call("scriptEnabled", { script: "game.demo.SetFlash", enabled: true });
                if (toggle.error) throw new Error("try MaterialOverride action: " + toggle.error);
                await runningAfterRestart(previous);
                await inspect(component, fields(1, true));
                const reset = await call("reset");
                if (reset.error) throw new Error("try MaterialOverride reset: " + reset.error);
                await runningAfterRestart();
                await inspect(component, fields(0, false));
                // The old scripted pulse remains on the materials guide; test
                // it as well rather than losing this Stage 12 regression.
                await startTry(`${prefix}/render2d/materials.html`);
                const guidePose = await call("pose", { node: example.node });
                if (!guidePose.pose || guidePose.pose.length !== 2) throw new Error("material guide is missing the pulse instance");
            }
            const gameImage = async () => {
                await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
                const shot = PNG.sync.read(await page.locator(".try canvas").screenshot());
                const width = Math.floor(shot.width / 2), height = shot.height - 30;
                const data = Buffer.alloc(width * height * 4);
                for (let y = 0; y < height; y++)
                    shot.data.copy(data, y * width * 4, ((y + 30) * shot.width) * 4, ((y + 30) * shot.width + width) * 4);
                return { width, height, data };
            };
            const beforeOrder = example.visualOrder ? await gameImage() : null;
            if (example.field) {
                const previous = (await call("state")).state.restarts;
                const result = await call("set", { component, field: example.field, value: example.value });
                if (result.error) throw new Error(`try ${component}: ${result.error}`);
                const selector = `.try .st-fields .v[data-k$="component:${component}/${example.field}"]`;
                await page.waitForFunction(([sel, text, restarts]) => {
                    const cell = document.querySelector(sel);
                    const state = JSON.parse(globalThis.aegoDocs.call("state", "{}")).state;
                    return cell?.textContent?.trim() === text && cell.classList.contains("changed") && state?.restarts > restarts;
                }, [selector, example.text, previous], { timeout: 20000, polling: 100 });
            }
            if (example.visualOrder) {
                const edited = await gameImage();
                if (edited.width !== beforeOrder.width || edited.height !== beforeOrder.height ||
                    pixelmatch(beforeOrder.data, edited.data, null, edited.width, edited.height, { threshold: 0.05 }) < 100)
                    throw new Error(`try ${component}: changing order did not visibly affect the Game View`);
                const reset = await call("reset");
                if (reset.error) throw new Error(`try ${component} reset: ` + reset.error);
                await page.waitForFunction(([sel, value]) => {
                    const cell = document.querySelector(sel);
                    return cell?.textContent?.trim() === value && !cell.classList.contains("changed");
                }, [`.try .st-fields .v[data-k$="component:${component}/${example.field}"]`, example.initial], { timeout: 20000 });
                const restored = await gameImage();
                if (restored.width !== beforeOrder.width || restored.height !== beforeOrder.height ||
                    pixelmatch(beforeOrder.data, restored.data, null, restored.width, restored.height, { threshold: 0.05 }) > 99)
                    throw new Error(`try ${component}: Reset did not restore the Game View`);
            }
            if (example.animated) {
                const reset = await page.evaluate(() => {
                    const invoke = (m) => JSON.parse(globalThis.aegoDocs.call(m, "{}"));
                    const loaded = invoke("reset");
                    if (loaded.error) throw new Error(loaded.error);
                    return invoke("play");
                });
                if (reset.error || !reset.state?.paused || reset.state.frames !== 0)
                    throw new Error("try MaterialOverride: could not pause the reset scene");
                // A frame number is not an animation time: under different
                // render pacing, frames 12 and 50 can show the same phase.
                // Observe actual Flash and pause in the same page turn, then
                // check the real Game View without reducing its pixel limit.
                const sample = async (relation, threshold) => {
                    const resumed = await call("play");
                    if (resumed.error || !resumed.state?.running || resumed.state.paused)
                        throw new Error("try MaterialOverride: could not resume the pulse");
                    const handle = await page.waitForFunction(pauseAtMaterialFlash, { relation, threshold }, { timeout: 30000, polling: 16 });
                    const observed = await handle.jsonValue();
                    await handle.dispose();
                    await page.evaluate(() => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))));
                    const image = PNG.sync.read(await page.locator(".try canvas").screenshot());
                    const held = await call("state");
                    const inspection = await call("inspect", { component, fields: ["engine.MaterialOverride.FlashValue"] });
                    if (held.error || !held.state?.paused || held.state.frames !== observed.frames ||
                        held.state.restarts !== observed.restarts || inspection.error ||
                        inspection.inspection?.observables?.["engine.MaterialOverride.FlashValue"] !== observed.value)
                        throw new Error("try MaterialOverride: pulse did not stay paused for its image");
                    return { image, observed };
                };
                const low = await sample("atMost", 0.25), high = await sample("atLeast", 0.75);
                if (high.observed.frames <= low.observed.frames || high.observed.restarts !== low.observed.restarts)
                    throw new Error(`try MaterialOverride: pulse did not advance: ${JSON.stringify({ low: low.observed, high: high.observed })}`);
                const before = low.image, after = high.image;
                if (before.width !== after.width || before.height !== after.height)
                    throw new Error("try MaterialOverride: scene dimensions changed between samples");
                // Compare the left-hand scene, excluding the changing Inspector/toolbar UI; subtle linear-to-display changes can fall below the golden's 0.1 threshold.
                const width = Math.floor(before.width * 0.4), height = before.height;
                const crop = (image) => {
                    const data = Buffer.alloc(width * height * 4);
                    for (let y = 0; y < height; y++)
                        image.data.copy(data, y * width * 4, y * image.width * 4, (y * image.width + width) * 4);
                    return data;
                };
                const changed = pixelmatch(crop(before), crop(after), null, width, height, { threshold: 0.01 });
                if (changed < 100)
                    throw new Error(`try MaterialOverride: the scripted pulse did not visibly change the scene: ${changed} pixels, ${JSON.stringify({ low: low.observed, high: high.observed })}`);
                console.log(`try: ${prefix || "/en"} MaterialOverride pulse ${low.observed.value.toFixed(3)} -> ${high.observed.value.toFixed(3)}, ${changed} changed pixels`);
            }
            console.log(`try: ${prefix || "/en"} ${component} runs${example.field ? " and edits" : " with animation"}`);
        }
    }
}

const srv = await serve(site);
const base = `http://127.0.0.1:${srv.address().port}`;
const browser = await chromium.launch({
    executablePath,
    headless: true,
    args: ["--mute-audio", "--use-angle=swiftshader", "--enable-unsafe-swiftshader", "--ignore-gpu-blocklist"],
});
const page = await browser.newPage({ viewport: { width: 1280, height: 900 }, deviceScaleFactor: 1, colorScheme: "dark" });
if (opts.traffic === "true" || measureOnly) await installTrafficProbe(page);
page.on("pageerror", (e) => fail("page error: " + e.message));
const call = (m, p = {}) => page.evaluate(([m, p]) => JSON.parse(globalThis.aegoDocs.call(m, JSON.stringify(p))), [m, p]);
const cell = '.try .st-fields .v[data-k$="component:physics.BoxCollider2D/restitution"]';
const restitution = () => page.locator(cell).first().textContent();

try {
    if (opts["mesh-only"] === "true" || opts["components-only"] === "true") {
        await lightingAndMaterials();
    } else {
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
    if (measureOnly) {
        const traffic = await measureTrafficFrames(page, "try", ".try canvas", +(opts.warmup ?? 120), +(opts.frames ?? 120));
        const report = { mode: "measurement-only; functional gates not run", profile: "try", scene: "physics.Body2D", viewport: [1280, 900], DPR: 1, browserVersion: browser.version(), traffic };
        console.log("measurement " + JSON.stringify(report));
        if (opts.report) fs.writeFileSync(opts.report, JSON.stringify(report, null, 2) + "\n");
    } else {
    if (opts.traffic === "true") {
        const first = await page.evaluate(() => ({ frames: JSON.parse(globalThis.aegoDocs.call("state", "{}")).state.frames, traffic: globalThis.__aegoTraffic.begin(".try canvas") }));
        await sleep(3000);
        const last = await page.evaluate(() => ({ frames: JSON.parse(globalThis.aegoDocs.call("state", "{}")).state.frames, traffic: globalThis.__aegoTraffic.end() }));
        const report = { profile: "try", scene: "physics.Body2D", viewport: [1280, 900], DPR: 1, traffic: summarizeTraffic(first.traffic, last.traffic, last.frames - first.frames) };
        console.log("try traffic " + JSON.stringify(report));
        if (opts.report) {
            fs.mkdirSync(path.dirname(path.resolve(opts.report)), { recursive: true });
            fs.writeFileSync(opts.report, JSON.stringify(report, null, 2) + "\n");
        }
    }
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
    await focusAndTap();
    await normalMapping();
    await normalMapping("/ru/render2d/normal-mapping.html");
    await lightingAndMaterials();
    }
    }
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
console.log(opts["mesh-only"] === "true" ? "web try Mesh2D only: ok; other scenarios not run" : opts["components-only"] === "true" ? "web try Mesh2D and SpriteRenderer only: ok; other scenarios not run" : measureOnly ? "web try measurement complete; functional gates not run" : "web try: ok");
