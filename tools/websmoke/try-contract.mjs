// A line-oriented adapter for democheck. It drives the published docs page and
// the actual WASM Try runtime, rather than implementing a second scene runner.
// Stdout is reserved for protocol replies; diagnostics go to stderr.
import fs from "node:fs";
import http from "node:http";
import path from "node:path";
import readline from "node:readline";
import zlib from "node:zlib";
import {createRequire} from "node:module";
import { chromium, firefox } from "playwright-core";
import pngjs from "pngjs";

const require = createRequire(import.meta.url);
const options = Object.fromEntries(process.argv.slice(2).map(arg => {
    const [key, ...value] = arg.replace(/^--/, "").split("=");
    return [key, value.join("=")];
}));
if (!options.site) throw new Error("--site is required");
const root = path.resolve(options.site);
const mime = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".wasm": "application/wasm", ".json": "application/json" };
const server = http.createServer((req, res) => {
    try {
        const url = new URL(req.url, "http://127.0.0.1");
        let name = decodeURIComponent(url.pathname);
        if (name.endsWith("/")) name += "index.html";
        const file = path.resolve(root, "." + name);
        if (!file.startsWith(root + path.sep) || !fs.statSync(file).isFile()) throw new Error("not found");
        res.writeHead(200, { "Content-Type": mime[path.extname(file)] || "application/octet-stream" });
        fs.createReadStream(file).pipe(res);
    } catch {
        res.writeHead(404);
        res.end();
    }
});
await new Promise((resolve, reject) => server.listen(0, "127.0.0.1", resolve).on("error", reject));
const base = `http://127.0.0.1:${server.address().port}`;
const browsers = new Map();
const renderers = new Map();
async function browserFor(environment) {
    if (!["chromium", "firefox"].includes(environment)) throw new Error("unsupported browser environment");
    if (browsers.has(environment)) return browsers.get(environment);
    const type = environment === "chromium" ? chromium : firefox;
    const settings = {headless: environment !== "firefox" || process.env.AEGO_TRY_FIREFOX_HEADED !== "1"};
    const executable = environment === "chromium" ? (process.env.CHROMIUM || "/usr/bin/chromium") : process.env.FIREFOX;
    if (executable) settings.executablePath = executable;
    if (environment === "chromium") {
        const angle = process.env.AEGO_TRY_CHROMIUM_ANGLE || "swiftshader";
        if (!["swiftshader", "gl"].includes(angle)) throw new Error("unsupported Chromium ANGLE backend");
        settings.args = ["--mute-audio", "--use-gl=angle", `--use-angle=${angle}`, "--ignore-gpu-blocklist"];
        if (angle === "swiftshader") settings.args.push("--enable-unsafe-swiftshader");
    } else {
        settings.firefoxUserPrefs = {"webgl.force-enabled": true, "webgl.disabled": false, "media.volume_scale": "0.0"};
        if (process.env.MOZ_DISABLE_CONTENT_SANDBOX === "1") settings.firefoxUserPrefs["security.sandbox.content.level"] = 0;
    }
    const browser = await type.launch(settings);
    browsers.set(environment, browser);
    return browser;
}
let pages = [];
let contract;
let fields;
let environment;
let captureID = 0;
let observationID = 0;

function evidenceDirectory() {
    const root = process.env.AEGO_DEMOCHECK_EVIDENCE;
    if (!root) return null;
    const runRoot = contract.demoID.endsWith(".negative-observable") ? root + "-negative" : root;
    return path.join(runRoot, environment, contract.demoID);
}

async function closePages() {
    for (const { page } of pages) await page.close();
    pages = [];
}
async function invoke(page, method, params = {}) {
    const reply = await page.evaluate(([m, p]) => JSON.parse(globalThis.aegoDocs.call(m, JSON.stringify(p))), [method, params]);
    if (reply.error) throw new Error(`${method}: ${reply.error}`);
    return reply;
}
async function running(page) {
    // Browsers may suspend requestAnimationFrame in the background page. Each
    // language must really render its own runtime before its state is checked.
    await page.bringToFront();
    await page.waitForFunction(() => {
        const status = document.querySelector(".try .try-status");
        if (status?.classList.contains("error")) throw new Error(status.textContent);
        if (!globalThis.aegoDocs?.call) return false;
        const r = JSON.parse(globalThis.aegoDocs.call("state", "{}"));
        return r.state?.running && !r.state.paused && r.state.frames > 0 && r.control?.enabled && r.control.remaining === 0;
    }, null, { timeout: 120000, polling: 50 });
}
async function advance(page, frames, keys = []) {
    await invoke(page, "advance", {frames, keys});
    await running(page);
}
function normalizedPayload(p) {
    return {
        scene: p.scene || "", select: p.select || "", code: p.code || "",
        prefabs: Object.fromEntries(Object.entries(p.prefabs || {}).sort(([a], [b]) => a.localeCompare(b))),
        artifacts: p.artifacts || "",
    };
}
async function checkCameraSearch(page, lang) {
    if (contract.componentID !== "camera.CameraBrain2D") return;
    for (const query of ["camera.CameraBrain2D", "Camera Output Selector 2D", "Выбор выхода камеры 2D", "Camera Brain 2D", "Мозг камеры 2D"]) {
        await page.locator("#docs-search").fill(query);
        await page.waitForFunction(() => [...document.querySelectorAll("#results a")].some(a => new URL(a.href).pathname.replace(/\.html$/, "").endsWith("/ref/camera.CameraBrain2D")) && !document.getElementById("results").hidden, null, {timeout: 10000});
        // Results are grouped by category, so a related guide may precede the
        // reference. Check the heading attached to this exact component result.
        const section = await page.locator("#results").evaluate(root => {
            const target = [...root.querySelectorAll("a")].find(a => new URL(a.href).pathname.replace(/\.html$/, "").endsWith("/ref/camera.CameraBrain2D"));
            let row = target?.closest("li");
            while (row && !row.classList.contains("cat")) row = row.previousElementSibling;
            return row?.textContent || "";
        });
        if (section !== (lang === "ru" ? "Справочник" : "Reference")) throw new Error(`${lang}: component search category is missing or not localized: ${section}`);
    }
    await page.locator("#docs-search").fill("");
}
async function open(req) {
    await closePages();
    contract = req.contract;
    environment = req.environment || "chromium";
    captureID = 0;
    observationID = 0;
    if (!contract || !req.payload || !/^[A-Za-z0-9_.-]+$/.test(contract.componentID) || !/^[A-Za-z0-9_.-]+$/.test(contract.demoID) || [".", ".."].includes(contract.demoID)) throw new Error("invalid contract or component ID");
    fields = [...new Set(contract.actions.flatMap(a => a.expected.map(e => e.observable)))];
    const browser = await browserFor(environment);
    for (const lang of ["en", "ru"]) {
        const page = await browser.newPage({ viewport: { width: 1280, height: 900 }, deviceScaleFactor: 1, colorScheme: "dark" });
        const state = { page, lang, errors: [] };
        pages.push(state);
        page.on("pageerror", e => state.errors.push(e.message));
        const prefix = lang === "ru" ? "/ru" : "";
        const response = await page.goto(`${base}${prefix}/ref/${encodeURIComponent(contract.componentID)}.html`);
        if (!response?.ok()) throw new Error(`${lang}: component page is not available`);
        const published = await page.evaluate(() => {
            const b = document.querySelector(".try[data-scene]");
            if (!b) throw new Error("Try block missing");
            return {
                lang: document.documentElement.lang,
                payload: { scene: b.dataset.scene, select: b.dataset.select, code: b.dataset.code, prefabs: b.dataset.prefabs ? JSON.parse(b.dataset.prefabs) : {}, artifacts: b.dataset.artifacts },
                title: b.querySelector(".try-demo-title")?.textContent,
                description: b.querySelector(".try-demo-description")?.textContent,
                input: b.querySelector(".try-demo-input")?.textContent,
            };
        });
        if (published.lang !== lang) throw new Error(`${lang}: wrong page language ${published.lang}`);
        if (JSON.stringify(normalizedPayload(published.payload)) !== JSON.stringify(normalizedPayload(req.payload)))
            throw new Error(`${lang}: published Try payload does not match the registry`);
        for (const key of ["title", "description", "input"]) {
            if (published[key] !== contract.languages[lang]?.[key]) throw new Error(`${lang}: missing or stale ${key}`);
        }
        await checkCameraSearch(page, lang);
        // Lazy auto-start may remove the button before the click completes.
        const start = await page.$(".try .try-start");
        if (start) {
            try {
                await start.click({timeout: 3000});
            } catch (error) {
                const booting = await page.evaluate(() => !!document.querySelector(".try .try-progress, .try canvas") || !!globalThis.aegoDocs?.call);
                if (!booting) throw error;
            } finally {
                await start.dispose();
            }
        }
        await page.waitForFunction(() => {
            const status = document.querySelector(".try .try-status");
            if (status?.classList.contains("error")) throw new Error(status.textContent);
            if (!globalThis.aegoDocs?.call) return false;
            return JSON.parse(globalThis.aegoDocs.call("state", "{}")).state?.running;
        }, null, {timeout: 120000});
        await page.locator(".try canvas").click({position: {x: 80, y: 80}});
        await invoke(page, "control", {enabled: true});
        await invoke(page, "reset");
        // Bootstrap only; the gate's Reset performs the contract warmup.
        await advance(page, 4);
        if (!renderers.has(environment)) {
            renderers.set(environment, await page.evaluate(() => {
                const gl = document.querySelector(".try canvas").getContext("webgl2");
                if (!gl) throw new Error("actual Try WebGL2 context is unavailable");
                const debug = gl.getExtension("WEBGL_debug_renderer_info");
                return {version: gl.getParameter(gl.VERSION), renderer: gl.getParameter(debug ? debug.UNMASKED_RENDERER_WEBGL : gl.RENDERER)};
            }));
        }
    }
}
async function frame(page, lang) {
    await page.bringToFront();
    // Do not accept stale pixels when a page is not rendering. A timeout is a
    // failed probe, never a substitute screenshot or a successful environment.
    let timer;
    try {
        await Promise.race([
            page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)))),
            new Promise((_, reject) => { timer = setTimeout(() => reject(new Error("Game View did not render two animation frames within 30s")), 30000); }),
        ]);
    } finally {
        clearTimeout(timer);
    }
    const viewport = (await invoke(page, "viewport")).viewport;
    if (!viewport || ![viewport.x, viewport.y, viewport.width, viewport.height, viewport.canvasWidth, viewport.canvasHeight].every(Number.isFinite) || viewport.width <= 0 || viewport.height <= 0 || viewport.canvasWidth <= 0 || viewport.canvasHeight <= 0) throw new Error("runtime did not report a valid Game View rectangle");
    const screenshot = await page.locator(".try canvas").screenshot();
    const dir = evidenceDirectory();
    if (dir) {
        fs.mkdirSync(dir, {recursive: true});
        fs.writeFileSync(path.join(dir, `${lang}-${String(captureID++).padStart(3, "0")}.png`), screenshot);
    }
    const shot = pngjs.PNG.sync.read(screenshot);
    // The Go layout owns this rectangle; a guessed crop could include the
    // changing Inspector and produce a false visual PASS.
    const sx = shot.width / viewport.canvasWidth, sy = shot.height / viewport.canvasHeight;
    const left = Math.ceil(viewport.x * sx), top = Math.ceil(viewport.y * sy);
    const right = Math.floor((viewport.x + viewport.width) * sx), bottom = Math.floor((viewport.y + viewport.height) * sy);
    const width = right - left, height = bottom - top;
    if (width <= 0 || height <= 0 || left < 0 || top < 0 || right > shot.width || bottom > shot.height) throw new Error("Game View capture is outside the actual canvas");
    const data = Buffer.alloc(width * height * 4);
    for (let y = 0; y < height; y++)
        shot.data.copy(data, y * width * 4, ((y + top) * shot.width + left) * 4, ((y + top) * shot.width + right) * 4);
    return { width, height, pixels: data.toString("base64") };
}
async function observe() {
    if (pages.length !== 2) throw new Error("no open Try instance");
    let observed;
    const frames = [];
    for (const { page, lang, errors } of pages) {
        await page.bringToFront();
        if (errors.length) throw new Error(`${lang}: browser error: ${errors.join("; ")}`);
        const log = await invoke(page, "log");
        const faults = (log.log || []).filter(line => line.error).map(line => line.text);
        if (faults.length) throw new Error(`${lang}: Try runtime error: ${faults.join("; ")}`);
        const reply = await invoke(page, "inspect", { component: contract.componentID, fields, dependencies: contract.dependencies });
        if (!reply.inspection) throw new Error(`${lang}: runtime returned no inspection`);
        if (observed && JSON.stringify(observed) !== JSON.stringify(reply.inspection))
            throw new Error(`${lang}: runtime observation differs across localized pages`);
        observed = reply.inspection;
        if (contract.actions.some(a => a.visual)) frames.push(await frame(page, lang));
    }
    const result = {identity: contract.identity, ...observed, frames};
    const dir = evidenceDirectory();
    if (dir) {
        fs.mkdirSync(dir, {recursive: true});
        const runID = (process.env.AEGO_DEMOCHECK_RUN_ID || "") + (contract.demoID.endsWith(".negative-observable") ? "-negative" : "");
        const record = {schema: 1, runID, environment, demoID: contract.demoID, observation: result};
        fs.writeFileSync(path.join(dir, `observation-${String(observationID++).padStart(3, "0")}.json.gz`), zlib.gzipSync(Buffer.from(JSON.stringify(record)), {level: 1}));
    }
    return result;
}
async function action(req) {
    const toggle = req.action.scriptToggle;
    const field = req.action.field;
    const prefix = contract.componentID + ".";
    const value = req.action.value;
    const stringList = Array.isArray(value) && value.length <= 64 && value.every(item => typeof item === "string");
    if (toggle && (typeof toggle.script !== "string" || typeof toggle.enabled !== "boolean" || field || value !== undefined)) throw new Error("unsupported script attachment action");
    if (field && (!field.startsWith(prefix) || (!stringList && !["number", "string"].includes(typeof value)))) throw new Error("unsupported action or value");
    if (!toggle && !field && !req.action.keys?.length && !req.action.inputs?.length) throw new Error("action has no field, script toggle or gameplay input");
    for (const { page } of pages) {
        await page.bringToFront();
        if (toggle) await invoke(page, "scriptEnabled", toggle);
        if (field) await invoke(page, "set", {component: contract.componentID, field: field.slice(prefix.length), value});
        if ((field || toggle) && (req.action.keys?.length || req.action.inputs?.length)) await advance(page, contract.warmupFrames || 4);
        for (const step of req.action.inputs || []) await advance(page, step.frames, step.keys || []);
        if (!req.action.inputs?.length || req.action.frames || req.action.keys?.length) {
            await advance(page, req.action.frames || contract.warmupFrames || 4, req.action.keys || []);
        }
    }
}
async function reset(req) {
    if (req.action !== "reset") throw new Error("unsupported reset");
    for (const { page } of pages) {
        await page.bringToFront();
        await invoke(page, "reset");
        await advance(page, contract.warmupFrames || 4);
    }
}
try {
    const lines = readline.createInterface({ input: process.stdin, crlfDelay: Infinity });
    for await (const line of lines) {
        let req;
        try {
            req = JSON.parse(line);
            let result;
            switch (req.method) {
                case "open": result = await open(req); break;
                case "observe": result = await observe(); break;
                case "action": result = await action(req); break;
                case "versions": result = {node: process.version, playwright: require("playwright-core/package.json").version, browsers: Object.fromEntries([...browsers].map(([name, browser]) => [name, browser.version()])), renderers: Object.fromEntries(renderers)}; break;
                case "reset": result = await reset(req); break;
                case "close": result = await closePages(); break;
                default: throw new Error(`unknown method ${req.method}`);
            }
            process.stdout.write(JSON.stringify({ id: req.id, result: result ?? null }) + "\n");
        } catch (error) {
            process.stdout.write(JSON.stringify({ id: req?.id, error: error.stack || String(error) }) + "\n");
        }
    }
} finally {
    await closePages();
    for (const browser of browsers.values()) await browser.close();
    await new Promise(resolve => server.close(resolve));
}
