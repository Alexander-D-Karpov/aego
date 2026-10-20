"use strict";

(function () {
    const root = document.querySelector('meta[name="aego-root"]').content;
    const input = document.getElementById("search");
    const results = document.getElementById("results");
    const NAV_KEY = "aego.docs.groups";
    const SECTIONS = { start: "Getting started", guides: "Guides", reference: "Reference" };
    const RANK = { start: 0, guides: 1, reference: 2 };
    let index = null;
    let active = 0;
    let shown = [];

    document.querySelector(".menu").addEventListener("click", () => document.body.classList.toggle("nav-open"));

    function loadState() {
        try {
            return JSON.parse(localStorage.getItem(NAV_KEY)) || {};
        } catch (_) {
            return {};
        }
    }

    const state = loadState();
    for (const d of document.querySelectorAll(".side details.group")) {
        const id = d.dataset.group;
        if (d.dataset.current) d.open = true;
        else if (id in state) d.open = !!state[id];
        else d.open = id === "start";
        d.addEventListener("toggle", () => {
            state[id] = d.open;
            try {
                localStorage.setItem(NAV_KEY, JSON.stringify(state));
            } catch (_) {}
        });
    }
    const current = document.querySelector(".side a.current");
    if (current) current.scrollIntoView({ block: "center" });

    const tocLinks = [...document.querySelectorAll(".toc a")];
    if (tocLinks.length && "IntersectionObserver" in window) {
        const byId = new Map(tocLinks.map((a) => [decodeURIComponent(a.hash.slice(1)), a]));
        const obs = new IntersectionObserver(
            (entries) => {
                for (const e of entries) {
                    if (!e.isIntersecting) continue;
                    const a = byId.get(e.target.id);
                    if (!a) continue;
                    for (const x of tocLinks) x.classList.toggle("active", x === a);
                }
            },
            { rootMargin: "-60px 0px -70% 0px" },
        );
        for (const id of byId.keys()) {
            const el = document.getElementById(id);
            if (el) obs.observe(el);
        }
    }

    function score(item, q) {
        const t = item.t.toLowerCase();
        const id = item.i.toLowerCase();
        if (t === q) return 1000;
        if (t.startsWith(q)) return 800 - t.length;
        const at = t.indexOf(q);
        if (at >= 0) return 600 - at;
        const ai = id.indexOf(q);
        if (ai >= 0) return 400 - ai;
        let j = 0;
        for (let i = 0; i < t.length && j < q.length; i++) if (t[i] === q[j]) j++;
        return j === q.length ? 100 : 0;
    }

    function escapeHTML(s) {
        return s.replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" })[c]);
    }

    function render(items) {
        let html = "";
        let last = null;
        items.forEach((it, i) => {
            if (it.c !== last) {
                html += `<li class="cat">${escapeHTML(SECTIONS[it.c] || "Other")}</li>`;
                last = it.c;
            }
            html += `<li${i === active ? ' class="active"' : ""}><a href="${root}${it.u}">${escapeHTML(it.t)} <span class="kind">${escapeHTML(it.g || it.k)}</span><span class="summary">${escapeHTML(it.s || "")}</span></a></li>`;
        });
        results.innerHTML = html;
        results.hidden = items.length === 0;
    }

    async function search() {
        const q = input.value.trim().toLowerCase();
        if (!q) {
            results.hidden = true;
            return;
        }
        if (!index) index = await (await fetch(root + "search.json")).json();
        const top = index
            .map((it) => [score(it, q), it])
            .filter((p) => p[0] > 0)
            .sort((a, b) => b[0] - a[0] || a[1].t.length - b[1].t.length)
            .slice(0, 30)
            .map((p) => p[1]);
        shown = top.map((it, i) => [it, i]).sort((a, b) => (RANK[a[0].c] ?? 3) - (RANK[b[0].c] ?? 3) || a[1] - b[1]).map((p) => p[0]);
        active = 0;
        render(shown);
    }

    input.addEventListener("input", search);
    input.addEventListener("keydown", (e) => {
        if (e.key === "ArrowDown" || e.key === "ArrowUp") {
            e.preventDefault();
            active = Math.max(0, Math.min(shown.length - 1, active + (e.key === "ArrowDown" ? 1 : -1)));
            render(shown);
        } else if (e.key === "Enter" && shown[active]) {
            location.href = root + shown[active].u;
        } else if (e.key === "Escape") {
            results.hidden = true;
            input.blur();
        }
    });
    document.addEventListener("keydown", (e) => {
        if (e.key === "/" && document.activeElement !== input && !/INPUT|TEXTAREA/.test(document.activeElement.tagName)) {
            e.preventDefault();
            input.focus();
        }
    });
    document.addEventListener("click", (e) => {
        if (!e.target.closest(".search")) results.hidden = true;
    });
})();
