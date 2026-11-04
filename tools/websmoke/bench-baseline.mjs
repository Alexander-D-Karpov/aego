// A CI benchmark must have a complete, compatible baseline. Local direct
// benchmarks may still run without one and report that comparison was skipped.
export const exactBenchmarkMetrics = [
    "drawsPerFrame", "bridge.calls", "bridge.draws", "bridge.bufferUploads", "bridge.bufferKB",
    "bridge.texUploads", "bridge.texKB", "bridge.programs", "bridge.targets",
];

const at = (o, p) => p.split(".").reduce((v, k) => (v == null ? v : v[k]), o);

export function requiredCIGateOptionErrors(opts) {
    const errors = [];
    if (opts["update-baseline"] === "true") errors.push("cannot update the baseline while comparing it in CI");
    if (opts.profile === "false") errors.push("the allocation profile is required in CI");
    if (!(opts.browsers ?? "chromium,firefox").split(",").filter(Boolean).length) errors.push("no browsers selected");
    return errors;
}

export function requiredCIMeasurementErrors(results, browsers, { gpu, frames, runs }) {
    const errors = [];
    for (const name of browsers) {
        const result = results[name];
        const samples = result?.runs;
        if (!Array.isArray(samples) || samples.length !== runs ||
            samples.some(x => !x || x.frames !== frames || !Array.isArray(x.errors) || x.errors.length)) {
            errors.push(`${name}: missing or incomplete benchmark measurements`);
            continue;
        }
        if (gpu === "swiftshader" && samples.some(x => !/swiftshader/i.test(x.renderer || ""))) {
            errors.push(`${name}: actual renderer is not SwiftShader`);
        }
        for (const p of [...exactBenchmarkMetrics, "frameBytesPerFrame"]) {
            const value = at(result.summary, p);
            if (!Number.isFinite(value) || value < 0) errors.push(`${name}: missing or invalid measurement ${p}`);
        }
        const profile = result.summary?.allocs;
        if (!profile || profile.frames !== frames || !Number.isFinite(profile.frameObjects) ||
            profile.frameObjects < 0 || !Array.isArray(profile.sites)) {
            errors.push(`${name}: missing or invalid allocation profile`);
        }
    }
    return errors;
}

export function requiredCIBaselineErrors(baseline, browsers, { gpu, warmup, frames, dpr, maxdpr }) {
    if (!baseline || typeof baseline !== "object") return ["baseline is missing for this GPU mode"];
    const errors = [];
    const meta = baseline.meta || {};
    if (meta.gpu !== gpu || meta.viewport !== "1280x720" || meta.dpr !== dpr || meta.maxdpr !== maxdpr ||
        meta.warmup !== warmup || meta.frames !== frames || meta.measurementOnly !== false) {
        errors.push("baseline measurement settings do not match the CI benchmark");
    }
    for (const name of browsers) {
        const r = baseline.results?.[name];
        if (!r || !Array.isArray(r.runs) || r.runs.length < 5 ||
            r.runs.some(x => x.frames !== frames || !Array.isArray(x.errors) || x.errors.length ||
                (gpu === "swiftshader" && !/swiftshader/i.test(x.renderer || "")))) {
            errors.push(`${name}: baseline needs five valid samples for this GPU mode`);
            continue;
        }
        for (const p of [...exactBenchmarkMetrics, "frameBytesPerFrame"]) {
            const v = at(r.summary, p);
            if (!Number.isFinite(v) || v < 0) errors.push(`${name}: missing or invalid baseline metric ${p}`);
        }
        for (const sample of ["gaps", "work"]) {
            const samples = r.summary?.samples?.[sample];
            if (!Array.isArray(samples) || !samples.length || samples.some(x => !Number.isFinite(x) || x < 0)) {
                errors.push(`${name}: missing or invalid baseline ${sample} samples`);
            }
        }
    }
    return errors;
}
