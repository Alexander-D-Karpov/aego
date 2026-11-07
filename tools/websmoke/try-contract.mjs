// A line-oriented adapter for democheck. It drives the published docs page and
// the actual WASM Try runtime, rather than implementing a second scene runner.
// Stdout is reserved for protocol replies; diagnostics go to stderr.
import fs from "node:fs";
import http from "node:http";
import path from "node:path";
import readline from "node:readline";
import { chromium } from "playwright-core";
import pngjs from "pngjs";

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
const browser = await chromium.launch({
    executablePath: process.env.CHROMIUM || "/usr/bin/chromium",
    headless: true,
    args: ["--mute-audio", "--use-angle=swiftshader", "--enable-unsafe-swiftshader", "--ignore-gpu-blocklist"],
});
let pages = [];
let contract;
let fields;

async function closePages() {
    for (const { page } of pages) await page.close();
    pages = [];
}
async function invoke(page, method, params = {}) {
    const reply = await page.evaluate(([m, p]) => JSON.parse(globalThis.aegoDocs.call(m, JSON.stringify(p))), [method, params]);
    if (reply.error) throw new Error(`${method}: ${reply.error}`);
    return reply;
}
async function running(page, restartAfter = -1) {
    // Chromium may suspend requestAnimationFrame in the background page. Each
    // language must really render its own runtime before its state is checked.
    await page.bringToFront();
    await page.waitForFunction(n => {
        const status = document.querySelector(".try .try-status");
        if (status?.classList.contains("error")) throw new Error(status.textContent);
        if (!globalThis.aegoDocs?.call) return false;
        const r = JSON.parse(globalThis.aegoDocs.call("state", "{}"));
        return r.state?.running && !r.state.paused && r.state.frames >= 4 && r.state.restarts > n;
    }, restartAfter, { timeout: 120000, polling: 50 });
}
function normalizedPayload(p) {
    return {
        scene: p.scene || "", select: p.select || "", code: p.code || "",
        prefabs: Object.fromEntries(Object.entries(p.prefabs || {}).sort(([a], [b]) => a.localeCompare(b))),
        artifacts: p.artifacts || "",
    };
}
async function open(req) {
    await closePages();
    contract = req.contract;
    if (!contract || !req.payload || !/^[A-Za-z0-9_.-]+$/.test(contract.componentID)) throw new Error("invalid contract or component ID");
    fields = [...new Set(contract.actions.flatMap(a => a.expected.map(e => e.observable)))];
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
        await page.locator(".try .try-start").click();
        await running(page);
    }
}
async function frame(page) {
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
    const shot = pngjs.PNG.sync.read(await page.locator(".try canvas").screenshot());
    // Exclude the changing Inspector and toolbar. This is the Game View.
    const width = Math.floor(shot.width / 2), height = shot.height - 30;
    if (width <= 0 || height <= 0) throw new Error("empty Game View capture");
    const data = Buffer.alloc(width * height * 4);
    for (let y = 0; y < height; y++)
        shot.data.copy(data, y * width * 4, ((y + 30) * shot.width) * 4, ((y + 30) * shot.width + width) * 4);
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
        if (contract.actions.some(a => a.visual)) frames.push(await frame(page));
    }
    return { ...observed, frames };
}
async function action(req) {
    const field = req.action.field;
    const prefix = contract.componentID + ".";
    if (!field?.startsWith(prefix) || !["number", "string"].includes(typeof req.action.value)) throw new Error("unsupported action or value");
    for (const { page } of pages) {
        await page.bringToFront();
        const previous = (await invoke(page, "state")).state.restarts;
        await invoke(page, "set", { component: contract.componentID, field: field.slice(prefix.length), value: req.action.value });
        await running(page, previous);
    }
}
async function reset(req) {
    if (req.action !== "reset") throw new Error("unsupported reset");
    for (const { page } of pages) {
        await page.bringToFront();
        await invoke(page, "reset");
        await running(page);
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
    await browser.close();
    await new Promise(resolve => server.close(resolve));
}
