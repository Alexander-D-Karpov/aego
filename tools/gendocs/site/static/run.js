"use strict";

(function () {
    const root = document.querySelector('meta[name="aego-root"]').content;
    let booting = null;

    function load(src) {
        return new Promise((resolve, reject) => {
            const s = document.createElement("script");
            s.src = src;
            s.onload = resolve;
            s.onerror = () => reject(new Error("cannot load " + src));
            document.head.appendChild(s);
        });
    }

    function boot() {
        if (booting) return booting;
        booting = (async () => {
            await load(root + "play/wasm_exec.js");
            const go = new Go();
            const ready = new Promise((resolve) => (globalThis.aegoReady = resolve));
            const resp = fetch(root + "play/aego.wasm");
            const { instance } = WebAssembly.instantiateStreaming
                ? await WebAssembly.instantiateStreaming(resp, go.importObject)
                : await WebAssembly.instantiate(await (await resp).arrayBuffer(), go.importObject);
            go.run(instance);
            await ready;
            if (globalThis.aego.error) throw new Error(globalThis.aego.error);
        })();
        return booting;
    }

    function line(out, cls, text) {
        const li = document.createElement("li");
        if (cls) li.className = cls;
        li.textContent = text;
        out.appendChild(li);
    }

    async function run(block, button) {
        const out = block.querySelector(".output");
        const code = block.querySelector("pre code").textContent;
        out.hidden = false;
        out.textContent = "";
        button.disabled = true;
        try {
            line(out, "", "Loading the runtime…");
            await boot();
            out.textContent = "";
            const res = JSON.parse(globalThis.aego.call("run", JSON.stringify({ files: { "main.aes": code }, frames: 120 })));
            if (res.error) throw new Error(res.error);
            const errors = (res.diagnostics || []).filter((d) => d.severity === "error");
            for (const d of errors) line(out, "error", `${d.line}:${d.col} ${d.code} ${d.message}`);
            if (!errors.length) {
                for (const l of res.log || []) line(out, l.level === "ERROR" ? "error" : "", `[${l.frame}] ${l.text}`);
                for (const f of res.faults || []) line(out, "fault", `${f.script} stopped with ${f.code}: ${f.message}`);
                line(out, "done", `${res.frames} frames`);
            }
        } catch (e) {
            out.textContent = "";
            line(out, "error", e.message);
        } finally {
            button.disabled = false;
        }
    }

    for (const block of document.querySelectorAll(".snippet")) {
        const button = block.querySelector(".run");
        button.addEventListener("click", () => run(block, button));
    }
})();
