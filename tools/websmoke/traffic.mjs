// Optional measurement for the existing smoke runners. It does not change the
// wasm applications or claim a paired performance result. Install before boot
// because the WebGL backend retains bound method references.
export async function installTrafficProbe(page) {
    await page.addInitScript(() => {
        const records = new WeakMap();
        const probe = { active: null, gaps: [], work: [], last: 0 };
        const queries = new Set(["createQuery", "deleteQuery", "beginQuery", "endQuery", "getQuery", "getQueryParameter", "isQuery"]);
        const draws = new Set(["drawArrays", "drawArraysInstanced", "drawElements", "drawElementsInstanced", "drawRangeElements"]);
        function record(canvas) {
            let r = records.get(canvas);
            if (!r) {
                r = { calls: 0, queries: 0, draws: 0, bufferUploads: 0, bufferBytes: 0, renderer: "" };
                records.set(canvas, r);
            }
            return r;
        }
        function bytes(source, offset, length) {
            if (!source || typeof source.byteLength !== "number") return 0;
            const unit = source.BYTES_PER_ELEMENT ?? 1;
            const available = Math.max(0, source.byteLength - (offset ?? 0) * unit);
            return length > 0 ? Math.min(available, length * unit) : available;
        }
        const proto = globalThis.WebGL2RenderingContext?.prototype;
        if (proto) for (const name of Object.getOwnPropertyNames(proto)) {
            const descriptor = Object.getOwnPropertyDescriptor(proto, name);
            if (name === "constructor" || typeof descriptor.value !== "function") continue;
            const original = descriptor.value;
            Object.defineProperty(proto, name, { ...descriptor, value: function (...args) {
                const r = record(this.canvas);
                if (queries.has(name) || (name === "getParameter" && args[0] === 0x8fbb)) r.queries++;
                else r.calls++;
                if (draws.has(name)) r.draws++;
                if (name === "bufferSubData") {
                    r.bufferUploads++;
                    r.bufferBytes += bytes(args[2], args[3], args[4]);
                } else if (name === "bufferData" && typeof args[1] !== "number") {
                    r.bufferUploads++;
                    r.bufferBytes += bytes(args[1], args[3], args[4]);
                }
                const value = original.apply(this, args);
                if (name === "getParameter" && (args[0] === 0x9246 || (args[0] === 0x1f01 && !r.renderer))) r.renderer = String(value);
                return value;
            } });
        }
        const raf = requestAnimationFrame.bind(globalThis);
        globalThis.requestAnimationFrame = callback => raf(time => {
            const active = probe.active;
            const before = active ? record(active).calls : 0;
            const start = performance.now();
            callback(time);
            if (active && active === probe.active && record(active).calls > before) {
                probe.work.push(performance.now() - start);
                if (probe.last) probe.gaps.push(time - probe.last);
                probe.last = time;
            }
        });
        probe.begin = selector => {
            const canvas = document.querySelector(selector);
            if (!canvas) throw new Error("traffic probe canvas is missing: " + selector);
            probe.active = canvas;
            probe.gaps = []; probe.work = []; probe.last = 0;
            return { ...record(canvas), time: performance.now() };
        };
        probe.end = () => {
            if (!probe.active) throw new Error("traffic window was not started");
            const r = { ...record(probe.active), time: performance.now(), gaps: probe.gaps, work: probe.work };
            probe.active = null;
            return r;
        };
        globalThis.__aegoTraffic = probe;
    });
}

export function summarizeTraffic(first, last, frames) {
    if (!(frames > 0)) throw new Error("traffic window did not advance frames");
    const quantiles = values => {
        const sorted = [...values].sort((a, b) => a - b);
        const at = p => sorted.length ? sorted[Math.min(sorted.length - 1, Math.floor(sorted.length * p))] : null;
        return { samples: sorted.length, p50: at(.5), p95: at(.95), max: at(1) };
    };
    const out = { method: "after-only JavaScript WebGL2 interception; query calls reported separately; timing includes probe overhead and host contention", frames, windowMs: last.time - first.time, renderer: last.renderer };
    for (const key of ["calls", "queries", "draws", "bufferUploads", "bufferBytes"]) out[key + "PerFrame"] = (last[key] - first[key]) / frames;
    out.frameIntervalMs = quantiles(last.gaps);
    out.frameWorkMs = quantiles(last.work);
    return out;
}

// Uses the same completed engine-frame boundaries in both source checkouts.
// It measures only: normal smoke assertions remain in the default runner mode.
export async function measureTrafficFrames(page, profile, selector, warmup = 120, frames = 120) {
    if (!Number.isInteger(warmup) || warmup < 1 || !Number.isInteger(frames) || frames < 1) throw new Error("invalid traffic frame window");
    const result = await page.evaluate(({ profile, selector, warmup, frames }) => new Promise((resolve, reject) => {
        const count = profile === "playground"
            ? () => JSON.parse(globalThis.aego.call("poll", "{}")).stats.frames
            : () => JSON.parse(globalThis.aegoDocs.call("state", "{}")).state.frames;
        const pose = () => profile === "try" ? JSON.parse(globalThis.aegoDocs.call("pose", '{"node":"Box"}')).pose : null;
        const warmTo = count() + warmup;
        let first, initialPose;
        let stopped = false;
        const timeout = setTimeout(() => { stopped = true; reject(new Error("traffic frame window timed out")); }, 120000);
        const tick = () => {
            if (stopped) return;
            try {
                const current = count();
                if (!first && current >= warmTo) {
                    initialPose = pose();
                    first = { frames: current, traffic: globalThis.__aegoTraffic.begin(selector) };
                } else if (first && current >= first.frames + frames) {
                    const last = { frames: current, traffic: globalThis.__aegoTraffic.end() };
                    clearTimeout(timeout); stopped = true;
                    resolve({ first, last, initialPose, finalPose: pose() });
                    return;
                }
                requestAnimationFrame(tick);
            } catch (error) { clearTimeout(timeout); stopped = true; reject(error); }
        };
        requestAnimationFrame(tick);
    }), { profile, selector, warmup, frames });
    const measured = result.last.frames - result.first.frames;
    if (measured !== frames) throw new Error(`traffic frame window measured ${measured}, want exactly ${frames}`);
    return { ...summarizeTraffic(result.first.traffic, result.last.traffic, measured), warmupFrames: warmup,
        firstFrame: result.first.frames, lastFrame: result.last.frames, initialPose: result.initialPose, finalPose: result.finalPose };
}
