import fs from "node:fs";
import http from "node:http";
import os from "node:os";
import path from "node:path";
import { chromium, firefox } from "playwright-core";

const opts = Object.fromEntries(
    process.argv.slice(2).map((a) => {
        const [k, v] = a.replace(/^--/, "").split("=");
        return [k, v ?? "true"];
    }),
);
const site = path.resolve(opts.site ?? "../../site");
const out = path.resolve(opts.out ?? "../../.cache/web-bench.json");
const md = path.resolve(opts.md ?? out.replace(/\.json$/, ".md"));
const basePath = path.resolve(opts.baseline ?? "../../.cache/web-bench.base.json");
const gates = JSON.parse(fs.readFileSync(new URL("./bench-gates.json", import.meta.url)));
const browsers = (opts.browsers ?? "chromium,firefox").split(",").filter(Boolean);
const runs = +(opts.runs ?? 5);
const minTimedRuns = 5;
const warmup = +(opts.warmup ?? 120);
const frames = +(opts.frames ?? 600);
const gogc = (opts.gogc ?? "").split(",");
const hard = opts.gpu === "hard";
const dpr = +(opts.dpr ?? 1);
const maxdpr = opts.maxdpr ? +opts.maxdpr : null;
const headed = opts.headed === "true";
const reference = opts.reference === "true" || (hard && !!gates.reference.host && os.hostname() === gates.reference.host);
const replay = opts.replay ? fs.readFileSync(path.resolve(opts.replay)).toString("base64") : null;
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

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

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

function probe() {
    const m = (globalThis.__bench = { on: false, hold: false, held: [], last: 0, work: [], gaps: [], long: 0, longMs: 0 });
    const raf = window.requestAnimationFrame.bind(window);
    const wrap = (cb) => (t) => {
        const s = performance.now();
        cb(t);
        if (m.on) {
            m.work.push(performance.now() - s);
            if (m.last) m.gaps.push(t - m.last);
        }
        m.last = t;
    };
    window.requestAnimationFrame = (cb) => (m.hold ? (m.held.push(cb), 0) : raf(wrap(cb)));
    m.release = () => {
        m.hold = false;
        m.last = 0;
        for (const cb of m.held.splice(0)) raf(wrap(cb));
    };
    m.hide = (on) => {
        Object.defineProperty(document, "hidden", { configurable: true, get: () => on });
        Object.defineProperty(document, "visibilityState", { configurable: true, get: () => (on ? "hidden" : "visible") });
        document.dispatchEvent(new Event("visibilitychange"));
    };
    try {
        new PerformanceObserver((l) => {
            if (!m.on) return;
            for (const e of l.getEntries()) {
                m.long++;
                m.longMs += e.duration;
            }
        }).observe({ type: "longtask" });
    } catch {}
}

async function launch(name) {
    const gpu = hard ? ["--ignore-gpu-blocklist"] : ["--use-angle=swiftshader", "--enable-unsafe-swiftshader", "--ignore-gpu-blocklist"];
    if (name === "chromium") {
        return chromium.launch({
            executablePath,
            headless: !headed,
            args: ["--mute-audio", "--disable-background-timer-throttling", "--disable-renderer-backgrounding", "--disable-backgrounding-occluded-windows", ...gpu],
        });
    }
    if (name === "firefox") return firefox.launch({ headless: !headed });
    throw new Error("unknown browser " + name);
}

async function waitFrames(page, n, timeout = 180000) {
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

async function waitFor(fn, what, timeout) {
    const end = Date.now() + timeout;
    for (;;) {
        const v = await fn();
        if (v) return v;
        if (Date.now() > end) throw new Error("timed out waiting for " + what);
        await sleep(100);
    }
}

const stats = async (page) => JSON.parse(await page.evaluate(() => globalThis.aegoGame.stats()));

function quant(xs) {
    if (!xs.length) return { p50: null, p95: null, p99: null, max: null };
    const s = [...xs].sort((a, b) => a - b);
    const q = (p) => s[Math.min(s.length - 1, Math.floor(p * s.length))];
    return { p50: q(0.5), p95: q(0.95), p99: q(0.99), max: s[s.length - 1] };
}

function median(xs) {
    const v = xs.filter((x) => x != null).sort((a, b) => a - b);
    if (!v.length) return null;
    const m = v.length >> 1;
    return v.length % 2 ? v[m] : (v[m - 1] + v[m]) / 2;
}

function erfc(x) {
    const t = 1 / (1 + 0.3275911 * x);
    const y = t * (0.254829592 + t * (-0.284496736 + t * (1.421413741 + t * (-1.453152027 + t * 1.061405429))));
    return y * Math.exp(-x * x);
}

function mannWhitney(a, b) {
    const n1 = a.length;
    const n2 = b.length;
    if (!n1 || !n2) return 1;
    const all = a.map((v) => [v, 1]).concat(b.map((v) => [v, 0])).sort((x, y) => x[0] - y[0]);
    const n = n1 + n2;
    let r1 = 0;
    let ties = 0;
    for (let i = 0; i < n; ) {
        let j = i;
        while (j < n && all[j][0] === all[i][0]) j++;
        const rank = (i + j + 1) / 2;
        const t = j - i;
        ties += t * t * t - t;
        for (let k = i; k < j; k++) if (all[k][1]) r1 += rank;
        i = j;
    }
    const u = r1 - (n1 * (n1 + 1)) / 2;
    const sigma = Math.sqrt(((n1 * n2) / 12) * (n + 1 - ties / (n * (n - 1))));
    if (!sigma) return 1;
    return erfc(Math.max(0, (Math.abs(u - (n1 * n2) / 2) - 0.5) / sigma) / Math.SQRT2);
}

function per(a, b, n) {
    return n > 0 ? ((b ?? 0) - (a ?? 0)) / n : null;
}

function summarize(s0, s1, raw) {
    const n = s1.frames - s0.frames;
    const zones = {};
    for (const k of Object.keys(s1.zones || {})) zones[k] = per((s0.zones || {})[k], s1.zones[k], n);
    const bridge = {};
    if (s1.bridge) {
        for (const k of Object.keys(s1.bridge)) bridge[k] = per(s0.bridge[k], s1.bridge[k], n);
        bridge.bridgeMs = bridge.bridgeNs / 1e6;
        delete bridge.bridgeNs;
        bridge.bufferKB = bridge.bufferBytes / 1024;
        delete bridge.bufferBytes;
        bridge.texKB = bridge.texBytes / 1024;
        delete bridge.texBytes;
    }
    const gf = s1.gpuFrames - s0.gpuFrames;
    return {
        frames: n,
        renderer: s1.renderer,
        frameMs: quant(raw.gaps),
        workMs: quant(raw.work),
        slowPct: raw.gaps.length ? (raw.gaps.filter((g) => g > 33.4).length * 100) / raw.gaps.length : null,
        longTasks: raw.long,
        longTaskMs: raw.longMs,
        jsHeapMB: raw.jsHeap == null ? null : raw.jsHeap / 1048576,
        allocsPerFrame: per(s0.allocObjects, s1.allocObjects, n),
        bytesPerFrame: per(s0.allocBytes, s1.allocBytes, n),
        frameBytesPerFrame: per(s0.frameBytes, s1.frameBytes, n),
        gcCycles: s1.gc - s0.gc,
        heapMB: s1.heapBytes / 1048576,
        heapGoalMB: s1.heapGoal / 1048576,
        gpuMs: gf > 0 ? (s1.gpuNs - s0.gpuNs) / gf / 1e6 : null,
        drawsPerFrame: per(s0.draws, s1.draws, n),
        zones,
        bridge,
        samples: { work: raw.work, gaps: raw.gaps },
    };
}

async function scenarios(page) {
    const res = {};
    await page.focus("#game");
    const before = await stats(page);
    await page.evaluate(() => {
        const m = globalThis.__bench;
        m.hold = true;
        m.hide(true);
    });
    for (const k of ["KeyA", "KeyS", "KeyD"]) await page.keyboard.press(k);
    await sleep(2000);
    await page.evaluate(() => {
        const m = globalThis.__bench;
        m.hide(false);
        m.release();
    });
    await waitFrames(page, before.frames + 60, 30000);
    const after = await stats(page);
    res.hidden = { clampedS: after.clamped - before.clamped, keys: after.keys - before.keys };
    if (await page.evaluate(() => globalThis.aegoGame.loseContext(true))) {
        await waitFor(async () => (await stats(page)).lost, "the context loss", 10000);
        const lost = await stats(page);
        await page.evaluate(() => globalThis.aegoGame.loseContext(false));
        const back = await waitFor(async () => {
            const s = await stats(page);
            return !s.lost && s.restores >= 1 && s.frames > lost.frames + 30 && s;
        }, "the context restore", 30000);
        res.contextLoss = { restores: back.restores, clampedS: back.clamped - lost.clamped };
    } else {
        res.contextLoss = { skipped: "WEBGL_lose_context is not available" };
    }
    return res;
}

async function runOnce(browser, gc, i, profile = false) {
    const ctx = await browser.newContext({ viewport: { width: 1280, height: 720 }, deviceScaleFactor: dpr });
    const page = await ctx.newPage();
    const errors = [];
    page.on("pageerror", (e) => errors.push(e.message));
    await page.addInitScript(probe);
    await page.addInitScript((b64) => {
        if (b64) globalThis.aegoReplay = b64;
    }, replay);
    const q = new URLSearchParams({ seed: "1", bench: "1" });
    if (gc) q.set("gogc", gc);
    if (profile) q.set("memprofile", "1");
    const tracing = opts.trace === "true" && i === 0 && !gc && !profile;
    if (tracing) q.set("trace", "1");
    try {
        await page.goto(`${base}/demo/?${q}`);
        await page.waitForFunction(() => globalThis.aegoGame, null, { timeout: 180000, polling: 20 });
        if (maxdpr && !(await page.evaluate((r) => globalThis.aegoGame.pixelRatio(r), maxdpr))) errors.push("the window cannot limit its pixel ratio");
        const set = await page.evaluate(
            ([from, to]) => {
                const m = globalThis.__bench;
                return globalThis.aegoGame.window(
                    from,
                    to,
                    () => (m.on = true),
                    (a, b) => {
                        m.on = false;
                        m.window = [a, b];
                    },
                );
            },
            [warmup, warmup + frames],
        );
        if (!set) throw new Error(`the game passed frame ${warmup} before the measurement window was set`);
        await page.waitForFunction(
            () => {
                const err = document.getElementById("error");
                if (err && !err.hidden) throw new Error(err.textContent);
                return !!globalThis.__bench.window;
            },
            null,
            { timeout: 300000, polling: 100 },
        );
        const raw = await page.evaluate(() => {
            const m = globalThis.__bench;
            return { window: m.window, work: m.work, gaps: m.gaps, long: m.long, longMs: m.longMs, jsHeap: performance.memory ? performance.memory.usedJSHeapSize : null };
        });
        await page.evaluate(() => globalThis.aegoGame.lockstep(false));
        const [s0, s1] = raw.window.map((w) => JSON.parse(w));
        const r = summarize(s0, s1, raw);
        if (s1.ticks - s0.ticks !== r.frames) errors.push(`${s1.ticks - s0.ticks} ticks in ${r.frames} frames, the window is not in lockstep`);
        if (profile) {
            const a = await page.evaluate(() => globalThis.aegoGame.allocs());
            r.allocs = a ? JSON.parse(a) : null;
            if (!r.allocs) errors.push("the page returned no allocation profile");
        }
        if (tracing) {
            const t = await page.evaluate(() => globalThis.aegoGame.trace());
            if (t) fs.writeFileSync(out.replace(/\.json$/, `-${browser.browserType().name()}.trace.json`), t);
        }
        if (i === 0 && !gc && !profile) r.scenarios = await scenarios(page);
        r.errors = errors;
        return r;
    } finally {
        await ctx.close();
    }
}

function aggregate(list) {
    const scalar = (f) => median(list.map(f));
    const group = (f) => {
        const keys = [...new Set(list.flatMap((r) => Object.keys(f(r) || {})))];
        return Object.fromEntries(keys.map((k) => [k, scalar((r) => (f(r) || {})[k] ?? null)]));
    };
    const work = list.flatMap((r) => r.samples.work);
    const gaps = list.flatMap((r) => r.samples.gaps);
    const out = {
        renderer: list[0].renderer,
        frameMs: quant(gaps),
        workMs: quant(work),
        zones: group((r) => r.zones),
        bridge: group((r) => r.bridge),
        scenarios: list.find((r) => r.scenarios)?.scenarios ?? null,
        samples: { work, gaps },
    };
    for (const k of ["slowPct", "longTasks", "longTaskMs", "jsHeapMB", "allocsPerFrame", "bytesPerFrame", "frameBytesPerFrame", "gcCycles", "heapMB", "heapGoalMB", "gpuMs", "drawsPerFrame"]) {
        out[k] = scalar((r) => r[k]);
    }
    return out;
}

const at = (o, p) => p.split(".").reduce((v, k) => (v == null ? v : v[k]), o);

const exact = ["drawsPerFrame", "bridge.calls", "bridge.draws", "bridge.bufferUploads", "bridge.bufferKB", "bridge.texUploads", "bridge.texKB", "bridge.programs", "bridge.targets"];
const timed = [["frameMs", "gaps"], ["workMs", "work"]];

function unstable(key, r, bad) {
    for (const p of exact) {
        const v = [...new Set(r.runs.map((x) => at(x, p)))];
        if (v.length > 1) bad.push(`${key}: ${p} differs between runs (${v.map((x) => fmt(x, 3)).join(", ")}), the counter is not deterministic`);
    }
}

function regressions(key, base, r, bad) {
    const pct = gates.regressPct;
    const b = base.summary;
    const s = r.summary;
    for (const p of exact) {
        const h = at(s, p);
        const o = at(b, p);
        if (h == null || o == null) continue;
        if (h !== o) bad.push(`${key}: ${p} ${fmt(o, 3)} -> ${fmt(h, 3)}, the baseline has a different count; after an intended change run make web-bench-baseline`);
    }
    const fb = s.frameBytesPerFrame;
    const ob = b.frameBytesPerFrame;
    if (fb != null && ob != null && fb > ob + gates.frameBytesSlack) {
        bad.push(`${key}: Go bytes in the engine frame ${fmt(ob, 1)} -> ${fmt(fb, 1)} per frame, more than ${gates.frameBytesSlack} over the baseline`);
    }
    const n = Math.min(r.runs.length, base.runs?.length ?? 0);
    if (n < minTimedRuns) {
        console.log(`${key}: frame and work time not compared, ${n} run(s), needs ${minTimedRuns}`);
        return;
    }
    for (const [name, sample] of timed) {
        const hb = b.samples?.[sample] ?? [];
        const hs = s.samples[sample];
        const mb = median(hb);
        const mh = median(hs);
        if (mb == null || mh == null) continue;
        const delta = ((mh - mb) / mb) * 100;
        const p = mannWhitney(hb, hs);
        if (p < 0.05 && delta > pct) bad.push(`${key}: ${name} median ${mb.toFixed(2)} -> ${mh.toFixed(2)} ms (${delta.toFixed(1)}%, p=${p.toFixed(3)})`);
    }
}

function exempt(s) {
    return (gates.allowFrame ?? []).find((x) => s.stack.some((l) => l.includes(x.match)));
}

function frameAllocs(a) {
    let n = a.frameObjects;
    for (const s of a.sites) if (s.inFrame && exempt(s)) n -= s.objects;
    return n;
}

function gate(results, baseline) {
    const bad = [];
    for (const [key, r] of Object.entries(results)) {
        if (r.gogc) continue;
        const s = r.summary;
        if (s.allocs) {
            const n = frameAllocs(s.allocs);
            if (gates.maxFrameAllocs != null && n > gates.maxFrameAllocs) {
                bad.push(`${key}: ${n} allocations in the engine frame over ${s.allocs.frames} frames, the limit is ${gates.maxFrameAllocs}; the sites are in ${md}`);
            }
        } else if (gates.maxFrameAllocs != null && opts.profile !== "false") {
            bad.push(`${key}: no allocation profile`);
        }
        if (reference) {
            if (s.frameMs.p95 > gates.p95Ms) bad.push(`${key}: frame p95 ${s.frameMs.p95.toFixed(2)} ms, over ${gates.p95Ms} ms`);
            if (s.slowPct > gates.slowPct) bad.push(`${key}: ${s.slowPct.toFixed(2)}% of frames over 33 ms, over ${gates.slowPct}%`);
        }
        const sc = s.scenarios;
        if (sc) {
            if (sc.hidden.clampedS > gates.hiddenClampedS) bad.push(`${key}: a hidden page added ${sc.hidden.clampedS.toFixed(3)} s of clamped time`);
            if (sc.hidden.keys < 6) bad.push(`${key}: ${sc.hidden.keys} of 6 key events sent while hidden reached the game`);
            if (!sc.contextLoss.skipped && sc.contextLoss.clampedS > gates.hiddenClampedS) bad.push(`${key}: the context loss added ${sc.contextLoss.clampedS.toFixed(3)} s of clamped time`);
        }
        unstable(key, r, bad);
        const b = baseline?.results?.[key];
        if (b) regressions(key, b, r, bad);
        else if (baseline) console.log(`${key}: no baseline entry`);
    }
    return bad;
}

function shown(stack) {
    const own = stack.filter((l) => !l.startsWith("runtime."));
    return (own.length ? own : stack).slice(0, 5);
}

function sites(results) {
    const lines = [];
    for (const [key, r] of Object.entries(results)) {
        const a = r.summary.allocs;
        if (!a) continue;
        lines.push("", `### ${key}: allocation sites over ${a.frames} frames`, "", "| objects | bytes | engine frame | allowed | site |", "|---:|---:|:---:|---|---|");
        for (const s of a.sites) {
            const x = s.inFrame ? exempt(s) : null;
            lines.push(`| ${s.objects} | ${s.bytes} | ${s.inFrame ? (s.truncated ? "deep" : "yes") : ""} | ${x ? x.reason : ""} | ${shown(s.stack).join(" ← ")} |`);
        }
    }
    return lines.join("\n") + "\n";
}

function fmt(v, d) {
    return v == null || Number.isNaN(v) ? "—" : v.toFixed(d);
}

function table(results) {
    const keys = Object.keys(results);
    const rows = [
        ["frame p50, ms", (s) => s.frameMs.p50, 2],
        ["frame p95, ms", (s) => s.frameMs.p95, 2],
        ["frame p99, ms", (s) => s.frameMs.p99, 2],
        ["frames > 33 ms, %", (s) => s.slowPct, 2],
        ["work p50, ms", (s) => s.workMs.p50, 2],
        ["work p95, ms", (s) => s.workMs.p95, 2],
        ["long tasks", (s) => s.longTasks, 0],
        ["JS heap, MB", (s) => s.jsHeapMB, 1],
        ["Go allocs/frame", (s) => s.allocsPerFrame, 1],
        ["Go bytes/frame", (s) => s.bytesPerFrame, 0],
        ["Go bytes/frame in engine frame", (s) => s.frameBytesPerFrame, 1],
        ["GC cycles", (s) => s.gcCycles, 0],
        ["profile: allocs/frame", (s) => (s.allocs ? s.allocs.objects / s.allocs.frames : null), 2],
        ["profile: allocs/frame in engine frame", (s) => (s.allocs ? s.allocs.frameObjects / s.allocs.frames : null), 3],
        ["profile: in frame only by a deep stack", (s) => s.allocs?.truncatedObjects ?? null, 0],
        ["Go heap, MB", (s) => s.heapMB, 1],
        ["GPU, ms", (s) => s.gpuMs, 2],
        ["WebGL calls/frame", (s) => s.bridge.calls, 1],
        ["timer query calls/frame", (s) => s.bridge.queries, 2],
        ["draw calls/frame", (s) => s.bridge.draws, 1],
        ["buffer uploads/frame", (s) => s.bridge.bufferUploads, 1],
        ["buffer KB/frame", (s) => s.bridge.bufferKB, 1],
        ["texture uploads/frame", (s) => s.bridge.texUploads, 2],
        ["program switches/frame", (s) => s.bridge.programs, 1],
        ["target switches/frame", (s) => s.bridge.targets, 1],
        ["bridge, ms/frame", (s) => s.bridge.bridgeMs, 3],
        ["hidden: clamped, s", (s) => s.scenarios?.hidden.clampedS, 3],
        ["hidden: key events", (s) => s.scenarios?.hidden.keys, 0],
        ["context restores", (s) => s.scenarios?.contextLoss.restores, 0],
    ];
    const zones = [...new Set(keys.flatMap((k) => Object.keys(results[k].summary.zones)))].sort();
    for (const z of zones) rows.push([`≈allocs/frame: ${z}`, (s) => s.zones[z], 2]);
    const lines = [`| metric | ${keys.join(" | ")} |`, `|---|${keys.map(() => "---:").join("|")}|`];
    for (const [name, f, d] of rows) lines.push(`| ${name} | ${keys.map((k) => fmt(f(results[k].summary), d)).join(" | ")} |`);
    return lines.join("\n") + "\n";
}

const srv = await serve(site);
const base = `http://127.0.0.1:${srv.address().port}`;
const meta = {
    date: new Date().toISOString(),
    host: os.hostname(),
    cpu: os.cpus()[0]?.model ?? "",
    reference,
    gpu: hard ? "hardware" : "swiftshader",
    viewport: "1280x720",
    dpr,
    maxdpr,
    warmup,
    frames,
    runs,
    browsers: {},
};
const results = {};
for (const name of browsers) {
    let browser;
    try {
        browser = await launch(name);
    } catch (e) {
        fail(`${name}: cannot start (${e.message.split("\n")[0]})${name === "firefox" ? "; install it with npx playwright-core install firefox" : ""}`);
        continue;
    }
    meta.browsers[name] = browser.version();
    try {
        for (const gc of gogc) {
            const key = gc ? `${name} gogc=${gc}` : name;
            const list = [];
            for (let i = 0; i < runs; i++) {
                try {
                    const r = await runOnce(browser, gc, i);
                    for (const e of r.errors) fail(`${key} run ${i + 1}: page error: ${e}`);
                    list.push(r);
                    console.log(`${key} run ${i + 1}: frame p95 ${fmt(r.frameMs.p95, 2)} ms, ${fmt(r.allocsPerFrame, 1)} allocs/frame, ${fmt(r.bridge.calls, 0)} WebGL calls/frame`);
                } catch (e) {
                    fail(`${key} run ${i + 1}: ${e.message}`);
                }
            }
            if (!list.length) continue;
            results[key] = { browser: name, gogc: gc || null, runs: list.map(({ samples, ...r }) => r), summary: aggregate(list) };
            if (gc || opts.profile === "false") continue;
            try {
                const p = await runOnce(browser, gc, 0, true);
                for (const e of p.errors) fail(`${key} profile: ${e}`);
                if (p.allocs) {
                    results[key].summary.allocs = { frames: p.frames, ...p.allocs };
                    console.log(`${key} profile: ${p.allocs.objects} allocations, ${p.allocs.frameObjects} in the engine frame, over ${p.frames} frames`);
                }
            } catch (e) {
                fail(`${key} profile: ${e.message}`);
            }
        }
    } finally {
        await browser.close();
    }
}
srv.close();

const report = { meta, results };
fs.mkdirSync(path.dirname(out), { recursive: true });
fs.writeFileSync(out, JSON.stringify(report) + "\n");
const text = table(results) + sites(results);
fs.writeFileSync(md, text);
console.log("\n" + text);
console.log(`wrote ${out} and ${md}`);

const saved = fs.existsSync(basePath) ? JSON.parse(fs.readFileSync(basePath, "utf8")) : {};
const modes = saved.meta ? { [saved.meta.gpu]: saved } : saved;
const mode = meta.gpu + (dpr !== 1 ? "@dpr" + dpr : "") + (maxdpr ? "@max" + maxdpr : "");
if (opts["update-baseline"] === "true") {
    modes[mode] = { meta, results: { ...modes[mode]?.results, ...results } };
    fs.writeFileSync(basePath, JSON.stringify(modes) + "\n");
    console.log(`baseline ${basePath} updated for ${mode}`);
}
if (opts.gate === "true") {
    const baseline = modes[mode] ?? null;
    if (!baseline) console.log(`no ${mode} baseline in ${basePath}, run make web-bench-baseline; only absolute gates apply`);
    for (const b of gate(results, baseline)) fail(b);
}
if (failures.length) {
    console.error(`${failures.length} failure(s)`);
    process.exit(1);
}
console.log("web bench: ok");
