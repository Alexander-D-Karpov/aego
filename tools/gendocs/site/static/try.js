"use strict";

(function () {
    const blocks = [...document.querySelectorAll(".try[data-scene]")];
    if (!blocks.length) return;
    globalThis.aegoSoundDefault = true;
    const starters = new Map();
    const texts = new Map();
    let api = null;
    let active = null;
    let canvas = null;

    function boot(report) {
        return globalThis.aegoRuntime("try", "aego-docs.wasm", "aegoDocsReady", "aegoDocs", report);
    }

    function call(method, payload) {
        return JSON.parse(api.call(method, JSON.stringify(payload || {})));
    }

    function status(b, text, error) {
        const s = b.querySelector(".try-status");
        s.textContent = text;
        s.classList.toggle("error", !!error);
    }

    function setValue(cell, value) {
        if (cell.dataset.kind === "color") {
            cell.textContent = "";
            const sw = document.createElement("i");
            sw.className = "sw";
            sw.style.background = value;
            cell.append(sw, value);
        } else {
            cell.textContent = value;
        }
    }

    function show(b, text, values) {
        texts.set(b, text);
        if (!values) return;
        for (const cell of b.querySelectorAll(".st-fields .v[data-k]")) {
            if (!(cell.dataset.k in values)) continue;
            if (cell.dataset.orig === undefined) cell.dataset.orig = cell.textContent;
            const v = values[cell.dataset.k];
            if (cell.textContent !== v) setValue(cell, v);
            cell.classList.toggle("changed", v !== cell.dataset.orig);
        }
    }

    globalThis.aegoDocsChanged = (text, values) => {
        if (active) show(active, text, values ? JSON.parse(values) : null);
    };

    function progress(b) {
        const box = document.createElement("div");
        box.className = "try-progress";
        box.innerHTML = '<div class="label">Loading the engine</div><div class="bar"><i></i></div><div class="phase"></div>';
        const bar = box.querySelector(".bar i");
        const phase = box.querySelector(".phase");
        const label = box.querySelector(".label");
        const update = (s) => {
            if (active !== b) return;
            const pct = s.total ? Math.min(100, (s.got * 100) / s.total) : 0;
            if (s.phase === "download") {
                bar.style.width = pct + "%";
                phase.textContent = s.total ? `${globalThis.aegoMB(s.got)} of ${globalThis.aegoMB(s.total)}` : globalThis.aegoMB(s.got);
            } else if (s.phase === "cache") {
                label.textContent = "Starting the engine";
                phase.textContent = "From the browser cache";
            } else if (s.phase === "compile") {
                bar.style.width = "100%";
                label.textContent = "Compiling";
                phase.textContent = "";
            } else if (s.phase === "start") {
                label.textContent = "Starting the scene";
            }
        };
        return { box, update };
    }

    let soundTimer = 0;

    function audio() {
        if (typeof globalThis.aegoAudioStats !== "function") return null;
        try {
            return JSON.parse(globalThis.aegoAudioStats());
        } catch (_) {
            return null;
        }
    }

    function syncSound(b) {
        const btn = b.querySelector(".try-sound");
        const st = audio();
        if (!btn || !st || active !== b) return;
        const state = st.muted ? "muted" : st.running ? "on" : "blocked";
        if (btn.dataset.state === state && !btn.hidden) return;
        btn.hidden = false;
        btn.dataset.state = state;
        btn.setAttribute("aria-pressed", String(state === "on"));
        btn.querySelector("span").textContent = state === "on" ? "Sound on" : state === "muted" ? "Sound off" : "Turn sound on";
        btn.title = state === "on" ? "Mute the scene" : state === "muted" ? "Unmute the scene" : "The browser holds sound until you click the page";
    }

    function watchSound(b) {
        clearInterval(soundTimer);
        if (!b.querySelector(".try-sound")) return;
        syncSound(b);
        soundTimer = setInterval(() => syncSound(b), 400);
    }

    function release(b) {
        clearInterval(soundTimer);
        const sound = b.querySelector(".try-sound");
        if (sound) sound.hidden = true;
        b.classList.remove("active");
        const stage = b.querySelector(".try-stage");
        stage.textContent = "";
        for (const el of starters.get(b)) stage.appendChild(el);
        b.querySelector(".try-reset").disabled = true;
        texts.set(b, b.dataset.scene);
        for (const cell of b.querySelectorAll(".st-fields .v[data-k]")) {
            if (cell.dataset.orig !== undefined) setValue(cell, cell.dataset.orig);
            cell.classList.remove("changed");
        }
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
        const p = progress(b);
        if (!api) stage.appendChild(p.box);
        b.classList.add("active");
        try {
            api = await boot(p.update);
            p.box.remove();
            if (active !== b) return;
            const res = call("load", { scene: b.dataset.scene, select: b.dataset.select || "", code: b.dataset.code || "", prefabs: b.dataset.prefabs ? JSON.parse(b.dataset.prefabs) : {} });
            if (res.error) throw new Error(res.error);
            show(b, res.text, res.values);
            b.querySelector(".try-reset").disabled = false;
            watchSound(b);
            status(b, "Change a field on the right: the scene restarts with it. Click the view to send input.");
        } catch (e) {
            p.box.remove();
            status(b, e.message, true);
        }
    }

    function download(b) {
        const blob = new Blob([texts.get(b) || b.dataset.scene], { type: "text/plain" });
        const a = document.createElement("a");
        a.href = URL.createObjectURL(blob);
        a.download = (b.dataset.name || "scene") + ".aescene";
        document.body.appendChild(a);
        a.click();
        a.remove();
        setTimeout(() => URL.revokeObjectURL(a.href), 1000);
        status(b, "Saved " + a.download + "; open it from the editor's Scenes panel or File → Open");
    }

    for (const b of blocks) {
        starters.set(b, [...b.querySelector(".try-stage").childNodes]);
        texts.set(b, b.dataset.scene);
        b.querySelector(".try-start").addEventListener("click", () => activate(b));
        b.querySelector(".try-reset").addEventListener("click", () => {
            if (active !== b || !api) return;
            const res = call("reset");
            if (res.error) status(b, res.error, true);
            else show(b, res.text, res.values);
        });
        b.querySelector(".try-open").addEventListener("click", () => download(b));
        const sound = b.querySelector(".try-sound");
        if (sound) sound.addEventListener("click", () => {
            const st = audio();
            if (active !== b || !st) return;
            if (st.muted || st.running) globalThis.aegoAudioMute(!st.muted);
            setTimeout(() => syncSound(b), 50);
        });
        b.querySelector(".try-copy").addEventListener("click", async () => {
            try {
                await navigator.clipboard.writeText(texts.get(b));
                status(b, "Copied; paste it into a .aescene file in your project");
            } catch (_) {
                status(b, "The browser blocked the clipboard", true);
            }
        });
    }

    globalThis.aegoRuntimeCached("try", "aego-docs.wasm").then((warm) => {
        if (!warm || !("IntersectionObserver" in window)) return;
        const seen = new IntersectionObserver((list) => {
            for (const it of list) {
                if (!it.isIntersecting || active) continue;
                seen.disconnect();
                activate(it.target.closest(".try"));
                return;
            }
        }, { threshold: 0.35 });
        for (const b of blocks) seen.observe(b.querySelector(".try-stage"));
    });
})();
