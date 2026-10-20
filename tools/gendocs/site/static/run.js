"use strict";

(function () {
    const boot = () => globalThis.aegoRuntime("play", "aego.wasm", "aegoReady", "aego");

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
            const api = await boot();
            out.textContent = "";
            const res = JSON.parse(api.call("run", JSON.stringify({ files: { "main.aes": code }, frames: 120 })));
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
