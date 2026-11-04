import fs from "node:fs";
import http from "node:http";
import path from "node:path";
import crypto from "node:crypto";
import { PNG } from "pngjs";
const { chromium, firefox } = await import(process.env.PLAYWRIGHT_MODULE || "playwright-core");

const options = Object.fromEntries(process.argv.slice(2).map(arg => arg.replace(/^--/, "").split("=")));
const root = path.resolve(options.root);
const golden = path.resolve(options.golden ?? "../../tests/integration/testdata/particle-goldens");
const out = path.resolve(options.out ?? "../../.cache/particle-goldens/browser");
const collectAll = options["collect-all"] === "true";
const names = ["Dust", "Spark", "Smoke", "Rain", "Explosion"];
const mime = { ".html": "text/html", ".wasm": "application/wasm", ".js": "text/javascript", ".json": "application/json", ".css": "text/css" };
const sha = data => crypto.createHash("sha256").update(data).digest("hex");
const report = { wasmSHA: sha(fs.readFileSync(path.join(root, "game.wasm"))), seed:42, frames:60, versions:{}, results:[], errors:[] };
fs.mkdirSync(out, { recursive:true });
const server = http.createServer((req, res) => {
    const name = decodeURIComponent(new URL(req.url, "http://localhost").pathname).replace(/^\/+/, "") || "index.html";
    const file = path.resolve(root, name);
    if (!file.startsWith(root + path.sep) || !fs.existsSync(file) || !fs.statSync(file).isFile()) {
        res.writeHead(404).end(); return;
    }
    res.writeHead(200, { "content-type": mime[path.extname(file)] || "application/octet-stream" });
    fs.createReadStream(file).pipe(res);
});
await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));

function exactFrames() {
    const original = window.requestAnimationFrame.bind(window);
    const state = window.particleTest = { started:false, paused:false, held:[] };
    window.requestAnimationFrame = fn => {
        if (!state.started && window.aegoGame) {
            state.started = true;
            state.paused = true;
        }
        return state.paused ? (state.held.push(fn), 0) : original(fn);
    };
    state.until = frame => {
        const game = window.aegoGame;
        state.started = true;
        if (!game.window(game.frames() + 1, frame, () => {}, () => { state.paused = true; })) throw Error("missed frame " + frame);
        state.paused = false;
        for (const fn of state.held.splice(0)) original(fn);
    };
}
async function advance(page, frame) {
    await page.evaluate(n => window.particleTest.until(n), frame);
    await page.waitForFunction(n => window.particleTest.paused && window.aegoGame.frames() === n, frame, { polling:50, timeout:120000 });
}
function compare(ref, got, file) {
    if (got.width !== ref.width || got.height !== ref.height) throw Error("different drawing size");
    let active = 0, referenceActive = 0, capturedActive = 0, different = 0, maxDelta = 0;
    const coordinates = [];
    const diff = new PNG({width: got.width, height: got.height});
    for (let i = 0; i < got.data.length; i += 4) {
        let refEdge = 0, gotEdge = 0, delta = 0;
        for (let c = 0; c < 4; c++) {
            refEdge = Math.max(refEdge, Math.abs(ref.data[i+c] - ref.data[c]));
            gotEdge = Math.max(gotEdge, Math.abs(got.data[i+c] - got.data[c]));
            delta = Math.max(delta, Math.abs(ref.data[i+c] - got.data[i+c]));
        }
        if (refEdge > 1) referenceActive++;
        if (gotEdge > 1) capturedActive++;
        if (refEdge > 1 || gotEdge > 1) active++;
        maxDelta = Math.max(maxDelta, delta);
        if (delta > 5) {
            different++;
            diff.data[i] = diff.data[i+3] = 255;
            if (coordinates.length < 128) coordinates.push({x: (i / 4) % got.width, y: Math.floor(i / (4 * got.width)), expected: [...ref.data.subarray(i,i+4)], actual: [...got.data.subarray(i,i+4)]});
        }
    }
    if (referenceActive < 6 || capturedActive < 6) throw Error("empty reference or capture: "+referenceActive+"/"+capturedActive+" pixels");
    const limit = Math.max(2, Math.floor(active/100));
    if (different > limit) {
        fs.writeFileSync(path.join(out,file+"-diff.png"),PNG.sync.write(diff));
        fs.writeFileSync(path.join(out,file+"-expected.png"),PNG.sync.write(ref));
    }
    return { active, referenceActive, capturedActive, different, limit, maxDelta,
        passed: different <= limit, ...(different > limit ? {coordinates} : {}) };
}
async function snap(page, file) {
    const bounds = await page.locator("#game").boundingBox();
    if (!bounds || bounds.width !== 256 || bounds.height !== 192) throw Error("canvas bounds changed: "+JSON.stringify(bounds));
    const bytes = await page.screenshot({ clip:bounds, timeout:30000 });
    fs.writeFileSync(path.join(out, file+".png"), bytes);
    return PNG.sync.read(bytes);
}
try {
    for (const browserName of (options.browsers || "chromium").split(",")) {
        const ff = browserName === "firefox";
        if (!ff && browserName!=="chromium") throw Error("unknown browser "+browserName);
        const browser = await (ff ? firefox : chromium).launch({
            executablePath: ff ? process.env.FIREFOX : process.env.CHROMIUM || "/usr/bin/chromium",
            headless: !ff || !process.env.DISPLAY,
            firefoxUserPrefs: ff ? { "webgl.force-enabled":true, "webgl.disabled":false } : {},
            args: ff ? [] : ["--no-sandbox","--use-angle=swiftshader","--enable-unsafe-swiftshader","--ignore-gpu-blocklist"],
        });
        report.versions[browserName] = browser.version();
        let uninterrupted;
        try {
            for (const [index, name] of [...names, "Rain"].entries()) {
                const restoring = index === names.length;
                const context = await browser.newContext({ viewport:{width:256,height:192}, deviceScaleFactor:1 });
                try {
                    const page = await context.newPage();
                    await page.addInitScript(exactFrames);
                    await page.goto("http://127.0.0.1:"+server.address().port+"/?seed=42&bench=1&scene=scenes/"+name+".aescene");
                    await page.waitForFunction(() => {
                        const err = document.querySelector("#error");
                        if (err && !err.hidden) throw Error(err.textContent);
                        return !!window.aegoGame;
                    }, null, { polling:20, timeout:120000 });
                    await page.addStyleTag({content:"#mute,#fullscreen,#loader{display:none!important}"});
                    await advance(page, 60);
                    const before = await snap(page, browserName+"-"+name+(restoring?"-loss":"")+"-60");
                    const entry = {browser:browserName, name, ...compare(PNG.sync.read(fs.readFileSync(path.join(golden,name+".png"))),before,browserName+"-"+name+"-60"),
                        stats:await page.evaluate(() => JSON.parse(aegoGame.stats()))};
                    report.results.push(entry);
                    const fail = message => {
                        report.errors.push(browserName+" "+name+": "+message);
                        if (!collectAll) throw Error(message);
                    };
                    if (!entry.passed) fail("pixel mismatch: "+entry.different+"/"+entry.active+" active (limit "+entry.limit+")");
                    if (name==="Rain") {
                        if (restoring) {
                            await page.evaluate(() => {
                                const canvas = document.getElementById("game");
                                window.testContext = {lost:0, restored:0};
                                canvas.addEventListener("webglcontextlost",()=>testContext.lost++,{once:true});
                                canvas.addEventListener("webglcontextrestored",()=>testContext.restored++,{once:true});
                                if (!window.aegoGame.loseContext(true)) throw Error("missing WEBGL_lose_context");
                            });
                            await page.waitForFunction(()=>window.testContext.lost===1,null,{timeout:10000,polling:30});
                            await page.evaluate(()=>window.aegoGame.loseContext(false));
                            await page.waitForFunction(()=>window.testContext.restored===1,null,{timeout:30000,polling:30});
                        }
                        await advance(page,100);
                        const after=await snap(page,browserName+(restoring?"-restored":"-continuous")+"-100");
                        if (restoring) {
                            entry.after=await page.evaluate(()=>JSON.parse(aegoGame.stats()));
                            if (entry.after.lost || entry.after.restores<1) throw Error("device not restored");
                            entry.recovered=compare(uninterrupted,after,browserName+"-recovered-100");
                            if (!entry.recovered.passed) fail("restored pixels differ");
                            if (sha(before.data)===sha(after.data)) throw Error("effect did not advance");
                        } else uninterrupted=after;
                    }
                } catch (error) {
                    const message = browserName+" "+name+": "+error.message;
                    if (!report.errors.includes(message)) report.errors.push(message);
                    if (!collectAll) throw error;
                } finally { await context.close(); }
            }
        } finally { await browser.close(); }
    }
} finally {
    fs.writeFileSync(path.join(out,"results.json"),JSON.stringify(report,null,2)+"\n");
    await new Promise(resolve=>server.close(resolve));
}
if (report.errors.length) throw Error(report.errors.join("\n"));
