// Documentation layout regression: keep wide tables at the original measure,
// and contain horizontal scrolling inside their region rather than the page.
import fs from "node:fs";
import path from "node:path";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);
const { chromium, firefox } = require(process.env.AEGO_PLAYWRIGHT_MODULE || "playwright-core");
const args = Object.fromEntries(process.argv.slice(2).map((arg) => {
    const at = arg.indexOf("=");
    if (!arg.startsWith("--") || at < 0) throw new Error("Use --base=URL [--report=FILE] [--screenshots=DIR]");
    return [arg.slice(2, at), arg.slice(at + 1)];
}));
const base = new URL(args.base || "http://127.0.0.1:18647/");
if (!["http:", "https:"].includes(base.protocol) || !base.pathname.endsWith("/")) throw new Error("Expected an HTTP(S) site root ending in /");
if (args.report && fs.existsSync(args.report)) throw new Error("Use a fresh report path");
if (args.screenshots) fs.mkdirSync(args.screenshots);
const report = { scope: "Headless documentation layout only; muted audio, not runtime or physical GPU acceptance.", browsers: {}, cases: [], failures: [] };

try {
    for (const name of ["chromium", "firefox"]) {
        const browser = await ({ chromium, firefox }[name]).launch(name === "chromium"
            ? { executablePath: process.env.CHROMIUM || "/usr/bin/chromium", headless: true, args: ["--mute-audio"] }
            : { headless: true, firefoxUserPrefs: { "media.volume_scale": "0.0" } });
        report.browsers[name] = browser.version();
        try {
            for (const width of [390, 1366, 1920, 2560]) {
                const context = await browser.newContext({ viewport: { width, height: 900 }, colorScheme: width === 390 ? "light" : "dark" });
                const page = await context.newPage();
                for (const lang of ["", "ru/"]) {
                    const routes = ["physics/components", "formats/meta"];
                    if (width === 1920) routes.push("ref/render2d.PostProcess", "ref/engine.MaterialOverride");
                    for (const route of routes) {
                        const response = await page.goto(new URL(lang + route, base).href, { waitUntil: "networkidle" });
                        await page.evaluate(() => document.fonts.ready);
                        const state = await page.evaluate(() => {
                            const doc = document.querySelector(".doc");
                            const box = doc.getBoundingClientRect();
                            const style = getComputedStyle(doc);
                            const available = box.width - parseFloat(style.paddingLeft) - parseFloat(style.paddingRight);
                            const tables = [...doc.querySelectorAll(":scope > .table-scroll")].map((region) => {
                                const table = region.querySelector("table");
                                const rect = region.getBoundingClientRect();
                                const ruler = document.createElement("div");
                                ruler.style.cssText = "position:absolute;visibility:hidden;width:calc(var(--measure) + 16ch);max-width:none;height:0;";
                                ruler.style.font = getComputedStyle(table).font;
                                doc.appendChild(ruler);
                                const originalMeasure = ruler.getBoundingClientRect().width;
                                ruler.remove();
                                return { width: rect.width, expected: Math.min(available, originalMeasure), originalMeasure,
                                    left: rect.left, right: rect.right, clientWidth: region.clientWidth, scrollWidth: region.scrollWidth,
                                    overflowX: getComputedStyle(region).overflowX, focusable: region.tabIndex === 0 };
                            });
                            const toc = document.querySelector(".toc");
                            const tocBox = toc.getBoundingClientRect();
                            return { viewport: innerWidth, overflow: document.documentElement.scrollWidth > innerWidth + 1,
                                docLeft: box.left, docRight: box.right, available, tables,
                                tocVisible: getComputedStyle(toc).display !== "none", tocRight: tocBox.right,
                                build: document.querySelector('meta[name="aego-docs-build"]')?.content };
                        });
                        const errors = [];
                        if (response.status() !== 200) errors.push("HTTP " + response.status());
                        if (state.overflow) errors.push("document overflows horizontally");
                        if (state.tocVisible && state.tocRight > width + 1) errors.push("right TOC escapes the viewport");
                        if (route === "physics/components" && !state.tables.length) errors.push("physics table has no scroll region");
                        for (const table of state.tables) {
                            if (Math.abs(table.width - table.expected) > 1) errors.push("table region lost the original readable measure");
                            if (table.left < state.docLeft - 1 || table.right > state.docRight + 1) errors.push("table region escapes the article");
                            if (table.overflowX !== "auto" || !table.focusable) errors.push("wide table lost keyboard-accessible scrolling");
                        }
                        const item = { browser: name, width, lang: lang ? "ru" : "en", route, ...state, errors };
                        report.cases.push(item);
                        if (errors.length) report.failures.push(item);
                        if (args.screenshots && (width === 1920 || width === 390) && route === "physics/components") {
                            await page.screenshot({ path: path.join(args.screenshots, `${name}-${item.lang}-${width}.png`) });
                        }
                    }
                }
                await context.close();
            }
        } finally { await browser.close(); }
    }
} catch (error) {
    report.error = error.stack;
} finally {
    if (args.report) fs.writeFileSync(args.report, JSON.stringify(report, null, 2) + "\n", { flag: "wx" });
    console.log(JSON.stringify({ browsers: report.browsers, cases: report.cases.length, failures: report.failures.length, error: report.error }));
    if (report.error || report.failures.length) process.exitCode = 1;
}
