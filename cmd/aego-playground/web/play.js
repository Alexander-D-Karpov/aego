"use strict";

const $ = (id) => document.getElementById(id);
const source = $("source");
const highlight = $("highlight");
const marks = $("marks");
const statusEl = $("status");
const runBtn = $("run");
const examplesEl = $("examples");
const framesEl = $("frames");
const problemsEl = $("problems");
const problemCount = $("problem-count");
const consoleEl = $("console");
const FILE = "main.aes";
const STORE = "aego.playground.source";

let examples = [];
let timer = 0;

const starter = `package demo

import "aego/log"

script Hello {
\tfunc Ready() {
\t\tlog.Info("hello from the playground")
\t}
}
`;

function setStatus(text, error) {
    statusEl.textContent = text;
    statusEl.classList.toggle("error", !!error);
}

function call(method, payload) {
    const out = JSON.parse(globalThis.aego.call(method, JSON.stringify(payload || {})));
    if (out && !Array.isArray(out) && out.error) throw new Error(out.error);
    return out;
}

function escapeHTML(s) {
    return s.replace(/[&<>]/g, (c) => (c === "&" ? "&amp;" : c === "<" ? "&lt;" : "&gt;"));
}

function paint(text, tokens) {
    let html = "";
    let at = 0;
    for (const t of tokens) {
        if (t.s < at) continue;
        html += escapeHTML(text.slice(at, t.s));
        html += `<span class="t-${t.k}">${escapeHTML(text.slice(t.s, t.e))}</span>`;
        at = t.e;
    }
    html += escapeHTML(text.slice(at));
    highlight.innerHTML = html + "\n";
}

function lineHeight() {
    return parseFloat(getComputedStyle(source).lineHeight) || 21;
}

function offsetOf(text, line, col) {
    let off = 0;
    for (let l = 1; l < line; l++) {
        const nl = text.indexOf("\n", off);
        if (nl < 0) return text.length;
        off = nl + 1;
    }
    return Math.min(text.length, off + Math.max(0, col - 1));
}

function reveal(line, col) {
    const off = offsetOf(source.value, line, col);
    source.focus();
    source.setSelectionRange(off, off);
    source.scrollTop = Math.max(0, (line - 4) * lineHeight());
    syncScroll();
}

function showProblems(diags) {
    problemsEl.textContent = "";
    marks.textContent = "";
    const lh = lineHeight();
    const top = parseFloat(getComputedStyle(source).paddingTop) || 0;
    const errors = diags.filter((d) => d.severity === "error").length;
    problemCount.textContent = diags.length ? `${errors} error(s), ${diags.length - errors} other` : "";
    for (const d of diags) {
        const li = document.createElement("li");
        li.className = d.severity;
        const where = d.line ? `${d.line}:${d.col}` : "";
        li.innerHTML = `<span class="where">${where}</span><span class="code-id">${escapeHTML(d.code || "")}</span>${escapeHTML(d.message)}`;
        if (d.line) {
            li.classList.add("clickable");
            li.onclick = () => reveal(d.line, d.col);
            const m = document.createElement("div");
            m.className = `mark ${d.severity}`;
            m.style.top = `${top + (d.line - 1) * lh}px`;
            m.dataset.line = d.line;
            marks.appendChild(m);
        }
        problemsEl.appendChild(li);
    }
    syncScroll();
}

function files() {
    return { [FILE]: source.value };
}

function refresh() {
    const text = source.value;
    try {
        paint(text, call("tokens", { source: text }));
        showProblems(call("compile", { files: files() }));
    } catch (e) {
        setStatus(e.message, true);
    }
    try {
        localStorage.setItem(STORE, text);
    } catch (_) {}
}

function schedule() {
    paint(source.value, []);
    clearTimeout(timer);
    timer = setTimeout(refresh, 150);
}

function logLine(cls, html) {
    const li = document.createElement("li");
    if (cls) li.className = cls;
    li.innerHTML = html;
    consoleEl.appendChild(li);
}

function run() {
    clearTimeout(timer);
    refresh();
    const frames = Math.max(1, Math.min(3600, parseInt(framesEl.value, 10) || 120));
    setStatus("Running…");
    const started = performance.now();
    let res;
    try {
        res = call("run", { files: files(), frames });
    } catch (e) {
        setStatus(e.message, true);
        return;
    }
    consoleEl.textContent = "";
    showProblems(res.diagnostics || []);
    const errors = (res.diagnostics || []).filter((d) => d.severity === "error");
    if (errors.length) {
        logLine("fault", `Build failed: ${errors.length} error(s), see Problems`);
        setStatus("Build failed", true);
        return;
    }
    for (const l of res.log || []) {
        const head = `<span class="where">${l.frame}</span><span class="level ${l.level}">${l.level}</span>`;
        const text = l.text.replace(/^\n+|\n+$/g, "");
        const body = text.includes("\n") ? `<pre class="block">${escapeHTML(text)}</pre>` : escapeHTML(text);
        logLine("", head + body);
    }
    for (const f of res.faults || []) {
        const where = f.line ? ` at ${escapeHTML(f.file)}:${f.line}` : "";
        logLine("fault", `${escapeHTML(f.script)} stopped with ${escapeHTML(f.code)}${where}: ${escapeHTML(f.message)}`);
    }
    const ms = Math.round(performance.now() - started);
    logLine("done", `${res.frames} frames in ${ms} ms`);
    consoleEl.scrollTop = consoleEl.scrollHeight;
    setStatus(res.faults && res.faults.length ? `${res.faults.length} fault(s)` : "Done", !!(res.faults && res.faults.length));
}

function syncScroll() {
    highlight.scrollTop = source.scrollTop;
    highlight.scrollLeft = source.scrollLeft;
    marks.style.transform = `translateY(${-source.scrollTop}px)`;
}

function loadExample(id, push) {
    const ex = examples.find((e) => e.id === id);
    if (!ex) return false;
    source.value = ex.code.endsWith("\n") ? ex.code : ex.code + "\n";
    examplesEl.value = id;
    if (push) {
        const url = new URL(location.href);
        url.searchParams.set("example", id);
        history.replaceState(null, "", url);
    }
    refresh();
    return true;
}

function indent(e) {
    if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
        e.preventDefault();
        run();
        return;
    }
    if (e.key === "Enter" && !e.altKey && !e.isComposing) {
        e.preventDefault();
        newline();
        return;
    }
    if (e.key !== "Tab" || e.shiftKey) return;
    e.preventDefault();
    insert("\t", 1);
}

function insert(text, caret) {
    const s = source.selectionStart;
    source.focus();
    if (!document.execCommand("insertText", false, text)) {
        source.setRangeText(text, s, source.selectionEnd, "end");
        schedule();
    }
    source.selectionStart = source.selectionEnd = s + caret;
}

function newline() {
    const { selectionStart: lo, selectionEnd: hi, value } = source;
    const ls = value.lastIndexOf("\n", lo - 1) + 1;
    const line = value.slice(ls, lo);
    const ws = line.match(/^[ \t]*/)[0];
    let text = "\n" + ws;
    let caret = text.length;
    if (line.trimEnd().endsWith("{")) {
        text += "\t";
        caret = text.length;
        if (value[hi] === "}") text += "\n" + ws;
    }
    insert(text, caret);
}

const loaderEl = $("loader");
const loaderTitle = $("loader-title");
const loaderBar = $("loader-bar");
const loaderDetail = $("loader-detail");

function mb(n) {
    return (n / 1048576).toFixed(1) + " MB";
}

function loading(title, detail) {
    loaderTitle.textContent = title;
    loaderDetail.textContent = detail || "";
}

function starting() {
    loaderTitle.textContent = "Starting engine…";
    loaderBar.style.width = "";
    loaderBar.classList.add("indeterminate");
}

async function fetchWasm(url) {
    const resp = await fetch(url);
    if (!resp.ok) throw new Error(`${url}: HTTP ${resp.status}`);
    const plain = !resp.headers.get("Content-Encoding");
    const total = (plain && parseInt(resp.headers.get("Content-Length"), 10)) || parseInt(loaderEl.dataset.wasmSize, 10) || 0;
    if (!resp.body) {
        starting();
        return resp;
    }
    if (total) loaderBar.classList.remove("indeterminate");
    let got = 0;
    const reader = resp.body.getReader();
    const body = new ReadableStream({
        async pull(ctl) {
            const { done, value } = await reader.read();
            if (done) {
                starting();
                ctl.close();
                return;
            }
            got += value.byteLength;
            if (total) {
                loaderBar.style.width = `${Math.min(100, (got / total) * 100)}%`;
                loaderDetail.textContent = `${mb(got)} of ${mb(total)}`;
            } else {
                loaderDetail.textContent = mb(got);
            }
            ctl.enqueue(value);
        },
    });
    return new Response(body, { headers: { "Content-Type": "application/wasm" } });
}

async function boot() {
    const go = new Go();
    const ready = new Promise((resolve) => (globalThis.aegoReady = resolve));
    loading("Loading engine…");
    const resp = await fetchWasm(`aego.wasm?v=${loaderEl.dataset.version || ""}`);
    const { instance } = WebAssembly.instantiateStreaming
        ? await WebAssembly.instantiateStreaming(resp, go.importObject)
        : await WebAssembly.instantiate(await resp.arrayBuffer(), go.importObject);
    go.run(instance);
    await ready;
    if (globalThis.aego.error) throw new Error(globalThis.aego.error);
    examples = call("examples");
    for (const ex of examples) {
        const o = document.createElement("option");
        o.value = ex.id;
        o.textContent = ex.title;
        examplesEl.appendChild(o);
    }
    const wanted = new URL(location.href).searchParams.get("example");
    if (!(wanted && loadExample(wanted, false))) {
        let saved = null;
        try {
            saved = localStorage.getItem(STORE);
        } catch (_) {}
        source.value = saved || starter;
        refresh();
    }
    runBtn.disabled = false;
    setStatus("Ready");
    loaderEl.classList.add("done");
    setTimeout(() => loaderEl.remove(), 250);
}

source.addEventListener("input", schedule);
source.addEventListener("scroll", syncScroll);
source.addEventListener("keydown", indent);
document.addEventListener("keydown", (e) => {
    if ((e.ctrlKey || e.metaKey) && !e.altKey && e.key.toLowerCase() === "s") e.preventDefault();
});
runBtn.addEventListener("click", run);
$("clear").addEventListener("click", () => (consoleEl.textContent = ""));
examplesEl.addEventListener("change", () => examplesEl.value && loadExample(examplesEl.value, true));

boot().catch((e) => {
    setStatus("Cannot start the playground", true);
    loaderEl.classList.add("failed");
    loading("Cannot start the playground", e.message);
});
