"use strict";

(function () {
    function meta(name) {
        const m = document.querySelector(`meta[name="${name}"]`);
        return m ? m.content : "";
    }

    const root = meta("aego-root");
    const runtimes = new Map();
    let support = null;

    function script(src) {
        return new Promise((resolve, reject) => {
            const s = document.createElement("script");
            s.src = src;
            s.onload = resolve;
            s.onerror = () => reject(new Error("cannot load " + src));
            document.head.appendChild(s);
        });
    }

    function url(dir, file) {
        const version = meta("aego-" + dir);
        return new URL(root + dir + "/" + file + (version ? "?v=" + version : ""), location.href).href;
    }

    // seen marks a runtime this browser has started, by its versioned URL.
    function seen(u, mark) {
        try {
            const key = "aego-seen:" + u.split("?")[0];
            if (mark) localStorage.setItem(key, u);
            return localStorage.getItem(key) === u;
        } catch (_) {
            return false;
        }
    }

    // cached trusts the mark: versioned runtimes are immutable, and probing the HTTP cache would hold its lock on the same URL.
    function cached(u) {
        return Promise.resolve(seen(u, false));
    }

    // count reads one branch of the download for progress, reporting at most once per frame.
    async function count(stream, total, report) {
        const reader = stream.getReader();
        let got = 0;
        let queued = false;
        const flush = () => {
            queued = false;
            report({ phase: "download", got, total });
        };
        for (;;) {
            const { done, value } = await reader.read();
            if (done) break;
            got += value.byteLength;
            if (!queued) {
                queued = true;
                requestAnimationFrame(flush);
            }
        }
        report({ phase: "compile", got, total: total || got });
    }

    async function start(dir, file, readyHook, api, report) {
        const u = url(dir, file);
        const total = Number(meta("aego-" + dir + "-size")) || 0;
        if (!support) {
            const v = meta("aego-" + dir);
            support = script(root + dir + "/wasm_exec.js" + (v ? "?v=" + v : "")).catch((e) => {
                support = null;
                throw e;
            });
        }
        await support;
        const ready = new Promise((resolve) => (globalThis[readyHook] = resolve));
        const warm = seen(u, false);
        if (report) report({ phase: warm ? "cache" : "download", got: 0, total });
        let resp = await fetch(u);
        if (!resp.ok) throw new Error(`${file}: HTTP ${resp.status}`);
        if (report && !warm && resp.body) {
            const [compile, progress] = resp.body.tee();
            count(progress, total, report).catch(() => {});
            resp = new Response(compile, { headers: resp.headers });
        }
        const go = new Go();
        const { instance } = WebAssembly.instantiateStreaming
            ? await WebAssembly.instantiateStreaming(resp, go.importObject)
            : await WebAssembly.instantiate(await resp.arrayBuffer(), go.importObject);
        if (report) report({ phase: "start", got: total, total });
        go.run(instance);
        await ready;
        seen(u, true);
        const out = globalThis[api];
        if (!out) throw new Error(file + " did not start");
        if (out.error) throw new Error(out.error);
        return out;
    }

    globalThis.aegoRuntime = function (dir, file, readyHook, api, report) {
        const key = dir + "/" + file;
        if (!runtimes.has(key)) {
            const listeners = new Set();
            const p = start(dir, file, readyHook, api, (s) => {
                for (const fn of listeners) fn(s);
            });
            p.listeners = listeners;
            runtimes.set(key, p);
            p.catch(() => runtimes.delete(key));
        }
        const p = runtimes.get(key);
        if (report) p.listeners.add(report);
        return p;
    };

    globalThis.aegoRuntimeCached = function (dir, file) {
        return cached(url(dir, file));
    };

    globalThis.aegoMB = (n) => (n / 1048576).toFixed(1) + " MB";
})();
