"use strict";

(function () {
    const build = document.documentElement.dataset.build || "";
    const canvas = document.getElementById("game");
    const loader = document.getElementById("loader");
    const bar = document.getElementById("bar");
    const status = document.getElementById("status");
    const errorBox = document.getElementById("error");
    const full = document.getElementById("fullscreen");
    const mute = document.getElementById("mute");

    function syncMute() {
        const on = globalThis.aegoAudioMute();
        mute.classList.toggle("muted", on);
        mute.title = on ? "Unmute" : "Mute";
        mute.setAttribute("aria-label", mute.title);
    }
    const PARALLEL = 6;
    const started = performance.now();
    let total = 0;
    let got = 0;

    function mb(n) {
        return (n / 1048576).toFixed(1) + " MB";
    }

    function fail(msg) {
        loader.classList.remove("done");
        loader.classList.add("failed");
        status.textContent = "The game cannot start";
        errorBox.textContent = String(msg);
        errorBox.hidden = false;
        if (!loader.isConnected) document.body.appendChild(loader);
    }

    globalThis.aegoFail = fail;
    globalThis.aegoStarted = function () {
        window.aegoBoot = Math.round(performance.now() - started);
        loader.classList.add("done");
        setTimeout(() => {
            if (errorBox.hidden) loader.remove();
        }, 300);
        full.hidden = !canvas.requestFullscreen;
        if (typeof globalThis.aegoAudioMute === "function") {
            mute.hidden = false;
            syncMute();
        }
        canvas.focus();
    };

    full.addEventListener("click", () => {
        const done = canvas.requestFullscreen();
        if (done && done.catch) done.catch(() => {});
        canvas.focus();
    });
    mute.addEventListener("click", () => {
        globalThis.aegoAudioMute(!globalThis.aegoAudioMute());
        syncMute();
        canvas.focus();
    });
    document.addEventListener("fullscreenchange", () => {
        full.hidden = !!document.fullscreenElement;
    });

    function progress(n) {
        got += n;
        if (!total) return;
        bar.style.width = Math.min(100, (got * 100) / total) + "%";
        status.textContent = `${mb(got)} of ${mb(total)}`;
    }

    async function read(url) {
        const resp = await fetch(url);
        if (!resp.ok) throw new Error(`${url}: HTTP ${resp.status}`);
        if (!resp.body) {
            const all = new Uint8Array(await resp.arrayBuffer());
            progress(all.byteLength);
            return all;
        }
        const reader = resp.body.getReader();
        const parts = [];
        let size = 0;
        for (;;) {
            const { done, value } = await reader.read();
            if (done) break;
            parts.push(value);
            size += value.byteLength;
            progress(value.byteLength);
        }
        const out = new Uint8Array(size);
        let at = 0;
        for (const p of parts) {
            out.set(p, at);
            at += p.byteLength;
        }
        return out;
    }

    async function each(items, fn) {
        let next = 0;
        const worker = async () => {
            while (next < items.length) await fn(items[next++]);
        };
        await Promise.all(Array.from({ length: Math.min(PARALLEL, items.length) }, worker));
    }

    async function start() {
        const resp = await fetch(`data/files.json?v=${build}`);
        if (!resp.ok) throw new Error(`data/files.json: HTTP ${resp.status}`);
        const list = await resp.json();
        total = list.wasm + list.manifest.size + list.files.reduce((s, f) => s + f.size, 0);
        const wasm = read(`game.wasm?v=${build}`);
        const manifest = read(`data/${list.manifest.path}?v=${build}`);
        const files = {};
        await each(list.files, async (f) => {
            files[f.path] = await read(`data/${f.path}`);
        });
        globalThis.aegoData = { manifest: await manifest, files };
        const bytes = await wasm;
        status.textContent = "Starting…";
        bar.style.width = "";
        bar.classList.add("indeterminate");
        const go = new Go();
        const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
        go.run(instance).then(() => {
            if (errorBox.hidden && !loader.classList.contains("done")) fail("the game exited during start-up");
        });
    }

    if (typeof WebAssembly !== "object") fail("this browser has no WebAssembly");
    else start().catch((e) => fail(e && e.message ? e.message : e));
})();
