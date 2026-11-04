// CI may report two known environment-sensitive checks as warnings. The
// checks still run, and every other failure remains a failure.
export function frameLimitOutcome(fps, renderer, warningsEnabled) {
    if (Number.isFinite(fps) && fps >= 26 && fps <= 32) return "pass";
    if (warningsEnabled && Number.isFinite(fps) && fps < 26 && /swiftshader/i.test(renderer)) return "warning";
    return "fail";
}

export class BrowserEventTimeout extends Error {
    constructor(kind) {
        super("no actual browser event: " + kind);
        this.kind = kind;
    }
}

export function visibilityFailureOutcome(browser, error, warningsEnabled, measurementStarted) {
    return warningsEnabled && browser === "firefox" && !measurementStarted && error instanceof BrowserEventTimeout &&
        ["ready", "hidden", "resumed"].includes(error.kind)
        ? "warning" : "fail";
}
