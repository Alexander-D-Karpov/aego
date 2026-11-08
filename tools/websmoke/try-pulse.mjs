// This predicate is serialized by Playwright and runs entirely in the page.
// Inspect and pause in the same JavaScript turn: another frame between those
// operations would make the captured image represent a different value.
export function pauseAtMaterialFlash({ relation, threshold }) {
    if (!Number.isFinite(threshold) || !["atMost", "atLeast"].includes(relation))
        throw new Error("invalid MaterialOverride pulse sample request");
    const invoke = (method, params = {}) => {
        const result = JSON.parse(globalThis.aegoDocs.call(method, JSON.stringify(params)));
        if (result.error) throw new Error(`${method}: ${result.error}`);
        return result;
    };
    const state = invoke("state").state;
    if (!state?.running || state.paused) throw new Error("MaterialOverride pulse stopped before sampling");
    if (state.frames === 0) return false;
    const request = { component: "engine.MaterialOverride", fields: ["engine.MaterialOverride.FlashValue"] };
    const inspection = invoke("inspect", request).inspection;
    const value = inspection?.observables?.["engine.MaterialOverride.FlashValue"];
    if (!inspection?.runtimeExecuting || typeof value !== "number" || !Number.isFinite(value))
        throw new Error("MaterialOverride pulse has no running Flash value");
    if (relation === "atMost" ? value > threshold : value < threshold) return false;
    const paused = invoke("play").state;
    const held = invoke("inspect", request).inspection;
    if (!paused?.running || !paused.paused || paused.frames !== state.frames || paused.restarts !== state.restarts ||
        held?.runtimeExecuting || held?.observables?.["engine.MaterialOverride.FlashValue"] !== value)
        throw new Error("MaterialOverride pulse changed while it was being paused");
    return { value, frames: paused.frames, restarts: paused.restarts };
}
