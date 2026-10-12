"use strict";

(function () {
    const root = document.querySelector('meta[name="aego-root"]').content;
    const input = document.getElementById("search");
    const results = document.getElementById("results");
    let index = null;
    let active = 0;

    document.querySelector(".menu").addEventListener("click", () => document.body.classList.toggle("nav-open"));

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
        results.innerHTML = items
            .map((it, i) => `<li${i === active ? ' class="active"' : ""}><a href="${root}${it.u}">${escapeHTML(it.t)} <span class="kind">${escapeHTML(it.k)}</span><span class="summary">${escapeHTML(it.s || "")}</span></a></li>`)
            .join("");
        results.hidden = items.length === 0;
    }

    let shown = [];
    async function search() {
        const q = input.value.trim().toLowerCase();
        if (!q) {
            results.hidden = true;
            return;
        }
        if (!index) index = await (await fetch(root + "search.json")).json();
        shown = index
            .map((it) => [score(it, q), it])
            .filter((p) => p[0] > 0)
            .sort((a, b) => b[0] - a[0] || a[1].t.length - b[1].t.length)
            .slice(0, 20)
            .map((p) => p[1]);
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
        if (e.key === "/" && document.activeElement !== input) {
            e.preventDefault();
            input.focus();
        }
    });
    document.addEventListener("click", (e) => {
        if (!e.target.closest(".search")) results.hidden = true;
    });
})();
