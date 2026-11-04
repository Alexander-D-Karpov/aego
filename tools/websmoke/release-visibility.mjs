// Run through release-visibility.sh. Browser automation protocols can force
// a page active. This gate launches ordinary headed browsers without a debugger.
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { randomUUID, createHash } from "node:crypto";
import fs from "node:fs";
import http from "node:http";
import os from "node:os";
import path from "node:path";
import { setTimeout as sleep } from "node:timers/promises";
import { BrowserEventTimeout, visibilityFailureOutcome } from "./ci-warning-policy.mjs";
import { launchVisibilityBrowser, waitForVisibilityBrowser, stopVisibilityBrowser, removeVisibilityProfile } from "./visibility-browser.mjs";

if (process.env.AEGO_PRIVATE_XVFB !== "1") throw new Error("use release-visibility.sh");
const opts = Object.fromEntries(process.argv.slice(2).map(v => v.replace(/^--/, "").split("=")));
if (!opts.root || !opts.out) throw new Error("--root and --out are required");
const root = path.resolve(opts.root);
const out = path.resolve(opts.out);
fs.mkdirSync(out, { recursive: true });
const limit = JSON.parse(fs.readFileSync(new URL("./bench-gates.json", import.meta.url))).hiddenClampedS;
const digests = Object.fromEntries(["game.wasm", "data/manifest.aetext"].map(name =>
    [name, createHash("sha256").update(fs.readFileSync(path.join(root, name))).digest("hex")]));
const report = { project: "examples/platformer", root, seed: 42, digests, browsers: [] };

function wrapper(title) {
    // The actual exported shell, loader, data and WASM are served unchanged.
    return `<!doctype html><title>${title}</title>
    <style>html,body,iframe{width:100%;height:100%;margin:0;border:0}</style>
    <iframe src="/?seed=42" id="game"></iframe><script>
    const w = document.getElementById("game").contentWindow;
    function report(kind,extra={}) { navigator.sendBeacon("/__event",
        JSON.stringify({kind,...extra,stats:JSON.parse(w.aegoGame.stats()),at:performance.now()})); }
    let interval = setInterval(()=>{
        if (!w.aegoGame || w.aegoGame.frames() < 10) return;
        clearInterval(interval);
        w.document.addEventListener("visibilitychange", e => {
            report(w.document.hidden ? "hidden" : "visible",{trusted:e.isTrusted});
            if (!w.document.hidden) {
                const frame=w.aegoGame.frames();
                if (!w.aegoGame.window(frame+1,frame+2,()=>queueMicrotask(()=>report("resumed")),()=>{})) {
                    report("resume-monitor-error");
                }
            }
        });
        const canvas=w.document.querySelector("canvas[data-aego]");
        report("ready",{viewport:[w.innerWidth,w.innerHeight],dpr:w.devicePixelRatio,drawingSize:[canvas.width,canvas.height]});
    },100);
    </script>`;
}

async function check(name, result) {
    const token = "AegoVisibility" + randomUUID().replaceAll("-", "");
    const events = result.events = [];
    const served = result.served = {};
    const server = http.createServer((req, res) => {
        const url = new URL(req.url, "http://localhost");
        if (url.pathname === "/__event" && req.method === "POST") {
            let body = "";
            req.setEncoding("utf8");
            req.on("data", part => { if (body.length < 65536) body += part; });
            req.on("end", () => {
                try { events.push(JSON.parse(body)); res.writeHead(204); }
                catch { res.writeHead(400); }
                res.end();
            });
            return;
        }
        if (url.pathname === "/__check") {
            res.writeHead(200, { "Content-Type": "text/html", "Cache-Control": "no-store" });
            res.end(wrapper(token)); return;
        }
        const relative = decodeURIComponent(url.pathname).replace(/^\/+/, "") || "index.html";
        const file = path.resolve(root, relative);
        if (!file.startsWith(root + path.sep) || !fs.existsSync(file) || !fs.statSync(file).isFile()) {
            res.writeHead(404); res.end(); return;
        }
        const types = { ".js": "text/javascript", ".html": "text/html", ".wasm": "application/wasm", ".json": "application/json" };
        res.writeHead(200, { "Content-Type": types[path.extname(file)] ?? "application/octet-stream", "Cache-Control": "no-store" });
        const stream = fs.createReadStream(file);
        if (Object.hasOwn(digests, relative)) {
            const hash = createHash("sha256");
            stream.on("data", bytes => hash.update(bytes));
            res.on("finish", () => { served[relative] = hash.digest("hex"); });
        }
        stream.on("error", err => res.destroy(err));
        stream.pipe(res);
    });
    await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));
    const url = `http://127.0.0.1:${server.address().port}/__check`;
    let dir;
    let browser;
    let stderr;
    let failure;
    try {
        dir = fs.mkdtempSync(path.join(os.tmpdir(), "aego-visible-"));
        const firefox = name === "firefox";
        const executable = firefox ? process.env.FIREFOX || "firefox" : process.env.CHROMIUM || "chromium";
        result.version = execFileSync(executable, ["--version"], { encoding: "utf8" }).trim();
        const args = firefox ? ["--no-remote", "--profile", dir, url]
            : [`--user-data-dir=${dir}`, "--no-first-run", "--no-default-browser-check", "--disable-sync",
               "--no-sandbox", "--disable-dev-shm-usage", "--use-angle=swiftshader", "--enable-unsafe-swiftshader",
               "--ignore-gpu-blocklist", "--disable-background-networking", "--window-size=1350,840", url];
        if (firefox) fs.writeFileSync(path.join(dir, "user.js"),
            'user_pref("webgl.force-enabled",true);\nuser_pref("browser.shell.checkDefaultBrowser",false);\nuser_pref("browser.startup.firstrunSkipsHomepage",false);\n');
        stderr = fs.openSync(path.join(out, name + "-browser.log"), "w");
        browser = launchVisibilityBrowser(executable, args, stderr);
        await waitForVisibilityBrowser(browser);
        async function wait(kind, after = 0) {
            const deadline = Date.now() + 120000;
            while (Date.now() < deadline) {
                if (browser.error) throw browser.error;
                if (browser.exited || browser.guard.exitCode !== null || browser.guard.signalCode) throw new Error("browser exited before " + kind);
                const e = events.slice(after).find(e => e.kind === kind);
                if (e) return e;
                await sleep(100);
            }
            throw new BrowserEventTimeout(kind);
        }
        const ready = await wait("ready");
        result.renderer = ready.stats.renderer;
        const xdo = (...args) => execFileSync("xdotool", args, { encoding: "utf8", timeout: 10000 }).trim();
        const win = xdo("search", "--sync", "--onlyvisible", "--name", token).split("\n").at(-1);
        // A fresh Firefox profile may open a Privacy Notice between the game
        // and the new tab. Clicking the launched game's first tab is an actual
        // browser action and does not assume that only two tabs exist.
        const showGameTab = () => {
            xdo("mousemove", "--window", win, "80", "20");
            xdo("click", "1");
        };
        xdo("windowfocus", "--sync", win);
        // Creating a fresh tab may stall the foreground frame. Open it once
        // before measurement; the actual check switches between two existing tabs.
        xdo("key", "--clearmodifiers", "ctrl+t");
        await wait("hidden");
        showGameTab();
        await wait("resumed");
        await sleep(1000);
        const start = events.length;
        result.measuredFromEvent = start;
        xdo("key", "--clearmodifiers", "ctrl+Tab");
        const hidden = await wait("hidden", start);
        await sleep(2500);
        showGameTab();
        const visible = await wait("visible", start);
        const resumed = await wait("resumed", start);
        assert(hidden.trusted && visible.trusted);
        assert(visible.at - hidden.at >= 2000, "hidden interval too short");
        result.activeSecondsWhileHidden = visible.stats.activeSeconds - hidden.stats.activeSeconds;
        result.clampedOnResume = resumed.stats.clamped - hidden.stats.clamped;
        assert(result.activeSecondsWhileHidden >= 0 && result.activeSecondsWhileHidden <= limit,
            "the engine platform clock counted the hidden interval");
        assert.equal(visible.stats.frames, hidden.stats.frames, "a hidden RAF ran the game");
        assert.equal(visible.stats.ticks, hidden.stats.ticks, "simulation advanced while hidden");
        assert.equal(resumed.stats.frames, visible.stats.frames + 1, "expected the first resumed frame");
        assert(resumed.stats.clamped - hidden.stats.clamped <= limit, "hidden time caused a catch-up");
        assert.equal(resumed.stats.postFXFailures, hidden.stats.postFXFailures);
        assert.deepEqual(served, digests);
    } catch (e) {
        failure = e;
    } finally {
        const cleanup = [];
        let stopped = !browser;
        if (browser) {
            try { await stopVisibilityBrowser(browser); stopped = true; }
            catch (e) { cleanup.push(e); }
        }
        if (stderr !== undefined) {
            try { fs.closeSync(stderr); } catch (e) { cleanup.push(e); }
        }
        try {
            server.closeAllConnections();
            await new Promise((resolve, reject) => server.close(e => e ? reject(e) : resolve()));
        } catch (e) { cleanup.push(e); }
        // Do not remove a profile while a browser descendant may still use it.
        if (dir && stopped) {
            try { await removeVisibilityProfile(dir); } catch (e) { cleanup.push(e); }
        }
        if (cleanup.length) throw new AggregateError(failure ? [failure, ...cleanup] : cleanup,
            "visibility browser cleanup failed: " + cleanup.map(String).join("; ") + (failure ? "; original: " + String(failure) : ""));
        if (failure) throw failure;
    }
}

for (const name of (opts.browsers ?? "chromium,firefox").split(",")) {
    if (!["chromium","firefox"].includes(name)) throw new Error("unknown browser: " + name);
    const result = { browser: name };
    report.browsers.push(result);
    try { await check(name, result); result.passed = true; }
    catch (error) {
        result.passed = false;
        result.failure = String(error);
        if (visibilityFailureOutcome(name, error, process.env.AEGO_CI_WEB_WARN === "1", result.measuredFromEvent !== undefined) === "warning") {
            result.warning = true;
            result.unverified = "Firefox did not complete visibility setup; hide/resume was not tested";
        } else process.exitCode = 1;
    }
    if (result.warning) {
        console.warn(`${name} WARN ${result.failure}; hide/resume was not tested`);
        if (process.env.GITHUB_ACTIONS) console.warn("::warning title=Firefox visibility::Setup timed out; hide/resume was not tested");
    } else console.log(name, result.passed ? "PASS" : result.failure);
    fs.writeFileSync(path.join(out, "results.json"), JSON.stringify(report, null, 2) + "\n");
}
