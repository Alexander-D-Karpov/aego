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
const mime = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".wasm": "application/wasm", ".json": "application/json" };
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

const srv = await serve(site);
const base = `http://127.0.0.1:${srv.address().port}`;
const browser = await chromium.launch({
    executablePath,
    headless: true,
    args: ["--use-angle=swiftshader", "--enable-unsafe-swiftshader", "--ignore-gpu-blocklist"],
});
const page = await browser.newPage({ viewport: { width: 1280, height: 900 }, deviceScaleFactor: 1 });
page.on("pageerror", (e) => fail("page error: " + e.message));
const call = (m, p = {}) => page.evaluate(([m, p]) => JSON.parse(globalThis.aegoDocs.call(m, JSON.stringify(p))), [m, p]);
const text = () => page.locator(".try .try-text code").first().textContent();

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
    await page.waitForFunction(() => document.querySelector(".try .try-text code").textContent.includes("restitution=0.75"), null, { timeout: 10000 });
    const st = (await call("state")).state;
    if (st.restarts < 1 || !st.running) fail(`the edit did not restart the scene: ${JSON.stringify(st)}`);
    await page.click(".try .try-reset");
    if ((await text()).includes("restitution=0.75")) fail("Reset kept the edit");
    const again = await page.evaluate(async () => {
        const a = await globalThis.aegoRuntime("try", "aego-docs.wasm", "aegoDocsReady", "aegoDocs");
        const b = await globalThis.aegoRuntime("play", "aego.wasm", "aegoReady", "aego");
        return a === globalThis.aegoDocs && JSON.parse(b.call("run", JSON.stringify({ files: { "main.aes": "package demo\n" }, frames: 1 }))).frames >= 1;
    });
    if (!again) fail("a second request did not reuse the runtimes");
    if (docsFetches !== 1) fail(`aego-docs.wasm was fetched ${docsFetches} times`);
    else console.log("try: edit, restart, reset and runtime reuse work");
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
