"use strict";

(function () {
    const meta = document.querySelector('meta[name="aego-root"]');
    const root = meta ? meta.content : "";
    const blocks = [...document.querySelectorAll(".try[data-scene]")];
    if (!blocks.length) return;
    const starters = new Map();
    let boot = null;
    let active = null;
    let canvas = null;

    function call(method, payload) {
        return JSON.parse(globalThis.aegoDocs.call(method, JSON.stringify(payload || {})));
    }

    function code(b) {
        return b.querySelector(".try-text code");
    }

    function status(b, text, error) {
        const s = b.querySelector(".try-status");
        s.textContent = text;
        s.classList.toggle("error", !!error);
    }

    function script(src) {
        return new Promise((resolve, reject) => {
            const s = document.createElement("script");
            s.src = src;
            s.onload = resolve;
            s.onerror = () => reject(new Error("cannot load " + src));
            document.head.appendChild(s);
        });
    }

    async function start() {
        await script(root + "try/wasm_exec.js");
        const ready = new Promise((resolve) => (globalThis.aegoDocsReady = resolve));
        const resp = await fetch(root + "try/aego-docs.wasm");
        if (!resp.ok) throw new Error("aego-docs.wasm: HTTP " + resp.status);
        const go = new Go();
        const { instance } = WebAssembly.instantiateStreaming
            ? await WebAssembly.instantiateStreaming(resp, go.importObject)
            : await WebAssembly.instantiate(await resp.arrayBuffer(), go.importObject);
        go.run(instance);
        await ready;
        if (globalThis.aegoDocs.error) throw new Error(globalThis.aegoDocs.error);
    }

    globalThis.aegoDocsChanged = (text) => {
        if (active) code(active).textContent = text;
    };

    function release(b) {
        b.classList.remove("active");
        const stage = b.querySelector(".try-stage");
        stage.textContent = "";
        for (const el of starters.get(b)) stage.appendChild(el);
        b.querySelector(".try-reset").disabled = true;
        code(b).textContent = b.dataset.scene;
        status(b, "");
    }

    async function activate(b) {
        if (active === b) return;
        if (active) release(active);
        active = b;
        if (!canvas) {
            canvas = document.createElement("canvas");
            canvas.setAttribute("data-aego", "");
            canvas.tabIndex = 0;
            canvas.setAttribute("aria-label", "Interactive scene with the Inspector");
        }
        const stage = b.querySelector(".try-stage");
        stage.textContent = "";
        stage.appendChild(canvas);
        b.classList.add("active");
        status(b, "Loading the engine…");
        try {
            if (!boot) boot = start();
            await boot;
            if (active !== b) return;
            const res = call("load", { scene: b.dataset.scene, select: b.dataset.select || "" });
            if (res.error) throw new Error(res.error);
            code(b).textContent = res.text;
            b.querySelector(".try-reset").disabled = false;
            status(b, "Change a field on the right: the scene restarts with it. Click the view to send input.");
        } catch (e) {
            if (!globalThis.aegoDocs) boot = null;
            status(b, e.message, true);
        }
    }

    for (const b of blocks) {
        starters.set(b, [...b.querySelector(".try-stage").childNodes]);
        b.querySelector(".try-start").addEventListener("click", () => activate(b));
        b.querySelector(".try-reset").addEventListener("click", () => {
            if (active !== b) return;
            const res = call("reset");
            if (res.error) status(b, res.error, true);
            else code(b).textContent = res.text;
        });
        b.querySelector(".try-copy").addEventListener("click", async () => {
            try {
                await navigator.clipboard.writeText(code(b).textContent);
                status(b, "Copied; paste it into a .aescene file in your project");
            } catch (_) {
                status(b, "The browser blocked the clipboard", true);
            }
        });
    }
})();
