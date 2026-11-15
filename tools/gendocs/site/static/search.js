"use strict";

// Kept independent of the DOM so browser and regression tests use one ranker.
globalThis.AegoDocsSearch = (() => {
    const normalize = (s) => (s || "").normalize("NFKC").toLowerCase().replace(/ё/g, "е").trim();

    function score(item, query) {
        const q = normalize(query);
        if (!q) return 0;
        const title = normalize(item.t);
        const id = normalize(item.i);
        const aliases = (item.a || []).map(normalize);
        const summary = normalize(item.s);
        const prose = normalize(item.x);
        let best = 0;
        if (id === q) best = 1400;
        else if (id.endsWith("." + q) || id.endsWith("/" + q)) best = 1200;
        else if (id.includes(q)) best = 400;
        if (title === q) best = Math.max(best, 1100);
        else if (title.startsWith(q)) best = Math.max(best, 800);
        else if (title.includes(q)) best = Math.max(best, 600);
        for (const alias of aliases) {
            if (alias === q) best = Math.max(best, 1000);
            else if (alias.startsWith(q)) best = Math.max(best, 750);
            else if (alias.includes(q)) best = Math.max(best, 550);
        }
        if (summary.includes(q)) best = Math.max(best, 250);
        if (prose.includes(q)) best = Math.max(best, 150);
        if (best) return best;

        const words = q.split(/\s+/);
        if (words.length > 1) {
            const labels = [title, id, ...aliases].join(" ");
            const description = summary + " " + prose;
            // Every word is required; do not flood phrase queries with partial hits.
            if (words.every((word) => labels.includes(word) || description.includes(word))) {
                return 160 + words.filter((word) => labels.includes(word)).length * 30;
            }
            return 0;
        }
        // Short arbitrary subsequences made "undo" find unrelated component names.
        // Restrict fuzzy fallback to substantial, same-initial title words.
        if (q.length < 5) return 0;
        for (const word of title.split(/[^\p{L}\p{N}_]+/u)) {
            if (word[0] !== q[0] || q.length / word.length < 0.6) continue;
            let j = 0;
            for (const c of word) if (c === q[j]) j++;
            if (j === q.length) return 50;
        }
        return 0;
    }

    function rank(index, query, limit = 30) {
        return index.map((item) => [score(item, query), item])
            .filter(([score]) => score > 0)
            .sort((a, b) => b[0] - a[0] || a[1].t.length - b[1].t.length || a[1].i.localeCompare(b[1].i))
            .slice(0, limit).map(([, item]) => item);
    }
    return { score, rank };
})();
