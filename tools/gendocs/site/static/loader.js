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

    async function start(dir, file, readyHook, api) {
        const version = meta("aego-" + dir);
        const query = version ? "?v=" + version : "";
        if (!support) {
            support = script(root + dir + "/wasm_exec.js" + query).catch((e) => {
                support = null;
                throw e;
            });
        }
        await support;
        const ready = new Promise((resolve) => (globalThis[readyHook] = resolve));
        const resp = await fetch(root + dir + "/" + file + query);
        if (!resp.ok) throw new Error(`${file}: HTTP ${resp.status}`);
        const go = new Go();
        const { instance } = WebAssembly.instantiateStreaming
            ? await WebAssembly.instantiateStreaming(resp, go.importObject)
            : await WebAssembly.instantiate(await resp.arrayBuffer(), go.importObject);
        go.run(instance);
        await ready;
        const out = globalThis[api];
        if (!out) throw new Error(file + " did not start");
        if (out.error) throw new Error(out.error);
        return out;
    }

    globalThis.aegoRuntime = function (dir, file, readyHook, api) {
        const key = dir + "/" + file;
        if (!runtimes.has(key)) {
            const p = start(dir, file, readyHook, api);
            runtimes.set(key, p);
            p.catch(() => runtimes.delete(key));
        }
        return runtimes.get(key);
    };
})();
