// Runs the existing measurement probes sequentially against two prepared sites.
// It never updates references/budgets and does not replace the functional gates.
import fs from "node:fs";
import path from "node:path";
import crypto from "node:crypto";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";

const [beforeArg, afterArg, outArg] = process.argv.slice(2);
if (!beforeArg || !afterArg || !outArg) throw new Error("usage: node profile-pair.mjs BEFORE_SITE AFTER_SITE OUTPUT_DIRECTORY");
const sites = { before: path.resolve(beforeArg), after: path.resolve(afterArg) };
const out = path.resolve(outArg);
const here = path.dirname(fileURLToPath(import.meta.url));
const replay = path.resolve(here, "../../tests/integration/testdata/platformer_run.aeinput");
const hash = p => crypto.createHash("sha256").update(fs.readFileSync(p)).digest("hex");
const artifacts = {};
for (const [phase, site] of Object.entries(sites)) {
    artifacts[phase] = {};
    for (const [profile, relative] of Object.entries({ runtime: "demo/game.wasm", playground: "play/aego.wasm", try: "try/aego-docs.wasm" })) {
        const file = path.join(site, relative);
        artifacts[phase][profile] = { file, sha256: hash(file), bytes: fs.statSync(file).size, gzipBytes: fs.statSync(file + ".gz").size };
    }
}
fs.mkdirSync(out, { recursive: true });
const manifest = { startedUTC: new Date().toISOString(), sites, artifacts, replay, replaySHA256: hash(replay),
    mode: "measurement-only; functional and allocation gates are separate", sequence: [],
    probes: Object.fromEntries(["profile-pair.mjs", "traffic.mjs", "bench.mjs", "smoke.mjs", "try.mjs"].map(name => [name, hash(path.join(here, name))])) };
try {
    for (const profile of ["runtime", "playground", "try"]) for (const phase of ["before", "after"]) {
        const dir = path.join(out, phase);
        fs.mkdirSync(dir, { recursive: true });
        const report = path.join(dir, profile + ".json"), log = path.join(dir, profile + ".log");
        const args = profile === "runtime"
            ? ["bench.mjs", `--site=${sites[phase]}`, `--out=${report}`, `--replay=${replay}`, "--browsers=chromium", "--runs=1", "--warmup=120", "--frames=600", "--profile=false", "--measure-only=true"]
            : [profile === "try" ? "try.mjs" : "smoke.mjs", `--site=${sites[phase]}`, `--report=${report}`, "--warmup=120", "--frames=120", "--measure-only=true"];
        const fd = fs.openSync(log, "w");
        const start = new Date().toISOString();
        let result;
        try { result = spawnSync(process.execPath, args, { cwd: here, env: process.env, stdio: ["ignore", fd, fd] }); }
        finally { fs.closeSync(fd); }
        manifest.sequence.push({ phase, profile, startUTC: start, endUTC: new Date().toISOString(), command: [process.execPath, ...args], report, log, exitCode: result.status, signal: result.signal });
        if (result.status !== 0) throw new Error(`${phase}/${profile} failed; see ${log}`);
        console.log(`${phase}/${profile}: measured`);
    }
} finally {
    manifest.finishedUTC = new Date().toISOString();
    fs.writeFileSync(path.join(out, "manifest.json"), JSON.stringify(manifest, null, 2) + "\n");
}
console.log(`paired observations saved in ${out}; no functional gate or statistical timing-regression claim`);
