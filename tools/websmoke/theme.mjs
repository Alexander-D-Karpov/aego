// Visual smoke of the site themes (D345): the landing page in the dark and the light theme, and the Auto/Dark/Light switch.
import fs from "node:fs";
import http from "node:http";
import path from "node:path";
import { chromium, firefox } from "playwright-core";

const opts = Object.fromEntries(
    process.argv.slice(2).map((a) => {
        const [k, v] = a.replace(/^--/, "").split("=");
        return [k, v ?? "true"];
    }),
);
const site = path.resolve(opts.site ?? "../../site");
const outDir = path.resolve(opts.out ?? "../../.cache/web-theme");
const executablePath = process.env.CHROMIUM || "/usr/bin/chromium";
const mime = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".wasm": "application/wasm", ".json": "application/json", ".png": "image/png", ".svg": "image/svg+xml", ".woff2": "font/woff2" };
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
            let file = path.join(root, p);
            if (!fs.existsSync(file) && fs.existsSync(file + ".html")) file += ".html";
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

const look = () => {
    const cs = getComputedStyle(document.body);
    return { bg: cs.backgroundColor, fg: cs.color, theme: document.documentElement.dataset.theme || "", mode: document.querySelector(".theme").dataset.mode };
};

const srv = await serve(site);
const base = `http://127.0.0.1:${srv.address().port}`;
const browser = opts.browser === "firefox" ? await firefox.launch({ headless: true }) : await chromium.launch({ executablePath, headless: true, args: ["--mute-audio"] });
fs.mkdirSync(outDir, { recursive: true });
try {
    const seen = {};
    for (const scheme of ["dark", "light"]) {
        const page = await browser.newPage({ viewport: { width: 1280, height: 900 }, deviceScaleFactor: 1, colorScheme: scheme });
        await page.goto(base + "/");
        await page.waitForLoadState("networkidle");
        seen[scheme] = await page.evaluate(look);
        if (seen[scheme].mode !== "auto") fail(`${scheme}: the switch starts at ${seen[scheme].mode}, want auto`);
        await page.screenshot({ path: path.join(outDir, `landing-${scheme}.png`) });
        await page.close();
    }
    if (seen.dark.bg === seen.light.bg || seen.dark.fg === seen.light.fg) fail(`the system theme does not change the page: ${JSON.stringify(seen)}`);

    const page = await browser.newPage({ viewport: { width: 1280, height: 900 }, colorScheme: "dark" });
    await page.goto(base + "/");
    const steps = [];
    for (let i = 0; i < 3; i++) {
        await page.click(".theme");
        steps.push(await page.evaluate(look));
    }
    const modes = steps.map((s) => s.mode).join(",");
    if (modes !== "dark,light,auto") fail(`the switch cycles ${modes}, want dark,light,auto`);
    if (steps[1].bg !== seen.light.bg) fail(`Light on a dark system gives ${steps[1].bg}, the light theme is ${seen.light.bg}`);
    await page.click(".theme");
    await page.reload();
    const kept = await page.evaluate(look);
    if (kept.mode !== "dark" || kept.theme !== "dark") fail(`the choice is not kept across a reload: ${JSON.stringify(kept)}`);
    console.log(`theme: dark ${seen.dark.bg} on ${seen.dark.fg}, light ${seen.light.bg} on ${seen.light.fg}; screenshots in ${outDir}`);
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
console.log("web theme: ok");
