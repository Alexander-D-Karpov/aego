// Run against a served standalone platformer with its normal controls and frame pacing.
// node tools/websmoke/menu-escape.mjs http://127.0.0.1:8000/
import assert from "node:assert/strict";
import { chromium } from "playwright-core";
import { installReleaseHashes, readReleaseHashes } from "./release-scenarios.mjs";

const url = new URL(process.argv[2]);
const browser = await chromium.launch({ executablePath: process.env.CHROMIUM || "/usr/bin/chromium",
    args: ["--no-sandbox", "--use-angle=swiftshader", "--enable-unsafe-swiftshader"] });
try {
    const page = await browser.newPage({ viewport: { width: 844, height: 390 },
        deviceScaleFactor: 1, isMobile: true, hasTouch: true });
    const touch = await page.context().newCDPSession(page);
    await installReleaseHashes(page);
    await page.addInitScript(() => {
        window.escapeEvents = [];
        for (const type of ["keydown", "keyup"]) {
            document.addEventListener(type, e => {
                if (e.code === "Escape") window.escapeEvents.push({ type, target: e.target.id, trusted: e.isTrusted });
            }, true);
        }
    });
    await page.goto(url.href);
    await page.waitForFunction(() => globalThis.aegoGame?.frames() >= 5, null, { timeout: 120000 });
    console.log(JSON.stringify({ served: await readReleaseHashes(page), browser: browser.version() }));
    const state = () => page.evaluate(() => ({
        frame: aegoGame.frames(), keys: JSON.parse(aegoGame.stats()).keys,
        open: aegoGame.menuOpen(), focus: document.activeElement.id, events: escapeEvents.splice(0),
    }));
    const pressEscape = async (expectedOpen, scenario) => {
        const before = await state();
        await page.keyboard.press("Escape");
        // Use the production smoke's deadline. Check the menu again after two game
        // frames so opening and immediately closing cannot look like a passing edge.
        await page.waitForTimeout(600);
        const after = await state();
        console.log(JSON.stringify({ scenario, before, after }));
        assert.equal(after.open, expectedOpen, scenario);
        assert(after.frame > before.frame, "no rendered frame after Escape");
        assert.equal(after.keys, before.keys + 2, "game did not receive one keydown and one keyup");
        assert.deepEqual(after.events, ["keydown", "keyup"].map(type => ({type, target: "game", trusted: true})));
        await page.waitForFunction(n => aegoGame.frames() >= n, after.frame + 2);
        assert.equal((await state()).open, expectedOpen, "menu changed again after the same key press");
    };
    for (const focus of ["game", "mute", "fullscreen"]) {
        await page.locator("#vpad .start").waitFor({state: "visible"});
        await page.locator("#" + focus).focus();
        const box = await page.locator("#vpad .start").boundingBox();
        try {
            await touch.send("Input.dispatchTouchEvent", {type: "touchStart", touchPoints: [
                {x: box.x + box.width / 2, y: box.y + box.height / 2, id: 2},
            ]});
            await page.waitForFunction(() => aegoGame.menuOpen(), null, {timeout: 5000});
        } finally {
            await touch.send("Input.dispatchTouchEvent", {type: "touchEnd", touchPoints: []});
        }
        await pressEscape(false, "Start then Escape, prior focus " + focus);
    }
    await pressEscape(true, "keyboard opens paused menu");
    await pressEscape(false, "second key press closes menu");
} finally {
    await browser.close();
}
