"use strict";

const assetV = (document.querySelector('meta[name="aego-assets"]') || {}).content || "";

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

    const menu = document.querySelector(".menu");
    const scrim = document.querySelector(".scrim");
    const doc = document.querySelector(".doc");
    const narrow = matchMedia("(max-width: 899px)");
    function setNav(open) {
        document.body.classList.toggle("nav-open", open);
        menu.setAttribute("aria-expanded", String(open));
        scrim.hidden = !open;
        doc.inert = open;
        document.body.style.overflow = open ? "hidden" : "";

    }
    menu.addEventListener("click", () => setNav(!document.body.classList.contains("nav-open")));
    scrim.addEventListener("click", () => setNav(false));
    document.querySelector(".side").addEventListener("click", (e) => {
        if (e.target.closest("a") && narrow.matches) setNav(false);
    });
    document.addEventListener("keydown", (e) => {
        if (e.key === "Escape" && document.body.classList.contains("nav-open")) setNav(false);
    });
    narrow.addEventListener("change", (e) => {
        if (!e.matches) setNav(false);
    });

    // The theme switch cycles Auto, Dark and Light; Auto follows the system (D345).
    const THEME_KEY = "aego.docs.theme";
    const MODES = ["auto", "dark", "light"];
    const LABELS = { auto: "Auto", dark: "Dark", light: "Light" };
    const prefersLight = matchMedia("(prefers-color-scheme: light)");
    const themeButton = document.querySelector(".theme");
    const effective = () => document.documentElement.dataset.theme || (prefersLight.matches ? "light" : "dark");
    function applyMode(mode) {
        if (mode === "auto") delete document.documentElement.dataset.theme;
        else document.documentElement.dataset.theme = mode;
        themeButton.dataset.mode = mode;
        themeButton.title = themeButton.ariaLabel = "Theme: " + LABELS[mode];
        globalThis.aegoTheme = effective();
        const media = mode === "auto" ? "(prefers-color-scheme: light)" : mode === "light" ? "all" : "not all";
        for (const s of document.querySelectorAll("picture.themed source")) s.media = media;
        document.dispatchEvent(new CustomEvent("aego-theme", { detail: globalThis.aegoTheme }));
    }
    let mode = "auto";
    try {
        const saved = localStorage.getItem(THEME_KEY);
        if (MODES.includes(saved)) mode = saved;
    } catch (_) {}
    applyMode(mode);
    themeButton.addEventListener("click", () => {
        mode = MODES[(MODES.indexOf(mode) + 1) % MODES.length];
        applyMode(mode);
        try {
            localStorage.setItem(THEME_KEY, mode);
        } catch (_) {}
    });
    prefersLight.addEventListener("change", () => {
        if (mode === "auto") applyMode("auto");
    });

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
        d.addEventListener("toggle", () => {
            state[id] = d.open;
            try {
                localStorage.setItem(NAV_KEY, JSON.stringify(state));
            } catch (_) {}
        });
    }
    const side = document.querySelector(".side");
    addEventListener("pagehide", () => {
        try {
            sessionStorage.setItem("aego.docs.side", String(side.scrollTop));
        } catch (_) {}
    });

    for (const pre of document.querySelectorAll(".doc pre")) {
        const code = pre.querySelector("code");
        if (!code || pre.classList.contains("sig")) continue;
        const btn = document.createElement("button");
        btn.type = "button";
        btn.className = "copy";
        btn.textContent = "Copy";
        btn.addEventListener("click", async () => {
            try {
                await navigator.clipboard.writeText(code.textContent);
                btn.textContent = "Copied";
            } catch (_) {
                btn.textContent = "Blocked";
            }
            setTimeout(() => (btn.textContent = "Copy"), 1200);
        });
        pre.appendChild(btn);
    }

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
            const ic = it.ic ? `<svg width="16" height="16" class="ic" aria-hidden="true"><use href="${root}assets/icons.svg?v=${assetV}#${it.ic}"/></svg>` : "<span></span>";
            html += `<li${i === active ? ' class="active"' : ""}><a href="${root}${it.u}">${ic}<span>${escapeHTML(it.t)}</span><span class="kind">${escapeHTML(it.g || it.k)}</span>${it.s ? `<span class="summary">${escapeHTML(it.s)}</span>` : ""}</a></li>`;
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

(function () {
    const demo = document.querySelector(".demo[data-src]");
    if (!demo) return;
    const play = () => {
        if (demo.querySelector("iframe")) return;
        const f = document.createElement("iframe");
        f.src = demo.dataset.src;
        f.title = "Aego demo";
        f.allow = "fullscreen; gamepad; autoplay";
        demo.textContent = "";
        demo.appendChild(f);
        f.addEventListener("load", () => f.focus());
    };
    const KEY = "aego-demo-played";
    demo.querySelector(".demo-play").addEventListener("click", () => {
        try {
            localStorage.setItem(KEY, "1");
        } catch (_) {}
        play();
    });
    try {
        if (localStorage.getItem(KEY)) play();
    } catch (_) {}
})();

(function () {
    const input = document.querySelector(".member-filter input");
    if (!input) return;
    const items = [...document.querySelectorAll("table.api tr[data-name], section.member[data-name]")];
    input.addEventListener("input", () => {
        const q = input.value.trim().toLowerCase();
        for (const el of items) el.classList.toggle("hidden-by-filter", q !== "" && !el.dataset.name.includes(q));
    });
})();

(function () {
    const done = new Set();
    const prefetch = (href) => {
        if (!href || done.has(href)) return;
        done.add(href);
        const l = document.createElement("link");
        l.rel = "prefetch";
        l.href = href;
        document.head.appendChild(l);
    };
    const next = document.querySelector(".pager a.next");
    if (next) prefetch(next.href);
    const local = (a) => a && a.origin === location.origin && a.pathname !== location.pathname && /(\/|\/[^./]+)$/.test(a.pathname);
    const hover = (e) => {
        const a = e.target.closest && e.target.closest("a[href]");
        if (local(a)) prefetch(a.href.split("#")[0]);
    };
    document.addEventListener("pointerover", hover, { passive: true });
    document.addEventListener("focusin", hover);
})();

(function () {
    const OS_NAMES = { windows: "Windows", macos: "macOS", linux: "Linux" };
    const ua = ((navigator.userAgentData && navigator.userAgentData.platform) || navigator.platform || navigator.userAgent).toLowerCase();
    const os = /win/.test(ua) ? "windows" : /mac|iphone|ipad/.test(ua) ? "macos" : /linux|x11/.test(ua) && !/android/.test(ua) ? "linux" : "";
    for (const box of document.querySelectorAll(".download")) {
        const main = box.querySelector(".dl-main");
        const pick = os && box.querySelector(`.dl-list li[data-os="${os}"] a`);
        if (pick) {
            main.href = pick.href;
            main.querySelector("span").textContent = `Download for ${OS_NAMES[os]}`;
            box.querySelector(".dl-file").textContent = pick.dataset.file;
            pick.classList.add("picked");
            pick.parentElement.classList.add("picked");
        } else {
            main.href = box.dataset.fallback;
            main.querySelector("span").textContent = os ? `Install on ${OS_NAMES[os]}` : "Install";
        }
    }
})();
