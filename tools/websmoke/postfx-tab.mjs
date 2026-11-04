// The additional pre-touch observation changes the timing of this diagnostic.
// Keep the unmodified demo.mjs run as the independent production gate.
import { createHash } from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";

if (!process.argv.some(arg => arg.startsWith("--diagnose-tab="))) throw Error("supply --diagnose-tab=/path/to/observations.json");
if (process.argv.some(arg => arg.startsWith("--update"))) throw Error("tab diagnostics cannot update image references");
const source = fs.readFileSync(new URL("./demo.mjs", import.meta.url), "utf8");
if (createHash("sha256").update(source).digest("hex") !== "1ea93f4582c046841014292057530212ea3988f04042b617ea631bbaf6a55ced") {
    throw Error("the official smoke changed; review the diagnostic instrumentation before use");
}
let code = source;
for (const dep of ["playwright-core", "pixelmatch", "pngjs"]) {
    code = code.replace('from "' + dep + '"', "from " + JSON.stringify(import.meta.resolve(dep)));
}
code = code.replace("async function frameLimit() {", `
const samples = [];
const tabState = () => page.evaluate(() => {
    const canvas = document.getElementById("game");
    return {time: performance.now(), frames: aegoGame.frames(), hidden: document.hidden,
            focus: document.hasFocus(), dpr: devicePixelRatio, buffer: [canvas.width, canvas.height]};
});
async function frameLimit(phase = "after-touch") {`);
code = code.replace("    const f0 =", "    const start = await tabState();\n    const f0 =");
code = code.replace("    if (fps < 26", `    const end = await tabState();
    samples.push({phase, start, end, originalFPS: fps, elapsedFPS: 1000 * (end.frames-start.frames)/(end.time-start.time)});
    fs.writeFileSync(opts["diagnose-tab"], JSON.stringify(samples, null, 2));
    if (phase === "before-touch") return;
    if (fps < 26`);
code = code.replace("    await touchPad();", '    await frameLimit("before-touch");\n    await touchPad();');
const dir = fs.mkdtempSync(path.join(os.tmpdir(), "aego-tab-probe-"));
try {
    const script = path.join(dir, "probe.mjs");
    fs.writeFileSync(script, code);
    const child = spawnSync(process.execPath, [script, ...process.argv.slice(2)], {stdio: "inherit"});
    if (child.error) throw child.error;
    process.exitCode = child.status ?? 1;
} finally {
    fs.rmSync(dir, {recursive: true, force: true});
}
