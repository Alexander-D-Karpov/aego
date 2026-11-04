"""Measure the cost of the camera stack on one immutable web release."""
import argparse
import json
import os
import platform
from datetime import datetime, timezone
import statistics
import subprocess
from pathlib import Path

from particles import HERE, ROUTE, ROUTE_SHA, fingerprint, sha


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("export", type=Path)
    parser.add_argument("out", type=Path)
    parser.add_argument("--gpu", choices=["soft", "hard"], default="soft")
    parser.add_argument("--expect-renderer", help="required for hardware, e.g. Iris")
    args = parser.parse_args()
    if args.gpu == "hard" and not args.expect_renderer:
        parser.error("supply --expect-renderer for a hardware run")
    root, out = args.export.resolve(), args.out.resolve()
    if root == out or root in out.parents:
        parser.error("measurement output must be outside the immutable export")
    identity = fingerprint(root)
    if sha(ROUTE) != ROUTE_SHA:
        parser.error("the canonical replay changed")
    out.mkdir(parents=True, exist_ok=False)
    report = {"export": identity, "replaySHA256": ROUTE_SHA, "seed": 42, "pairs": [], "complete": False}
    report.update(host=platform.node(), platform=platform.platform(), startUTC=datetime.now(timezone.utc).isoformat(),
                  probeSHA256=sha(HERE / "bench.mjs"), warmupFrames=120, measuredIntervals=600,
                  frameCap=0, mode="diagnostic lockstep, no allocation profiler", loadBefore=os.getloadavg() if hasattr(os, "getloadavg") else None)
    expect = args.expect_renderer or "swiftshader"
    try:
        for i in range(5):
            pair = {}
            report["pairs"].append(pair)
            for enabled in ([True, False] if i % 2 == 0 else [False, True]):
                name = "on" if enabled else "off"
                dest = out / f"{i+1}-{name}.json"
                command = ["node", str(HERE / "bench.mjs"), f"--root={root}", f"--out={dest}", "--md=false",
                           "--measure-only=true", "--exact-intervals=true", "--receipts=true", "--profile=false",
                           "--warmup=120", "--frames=600", "--runs=1", "--seed=42", f"--replay={ROUTE}",
                           "--browsers=chromium", f"--gpu={args.gpu}", f"--bool=render2d.postProcessing:{int(enabled)}"]
                with dest.with_suffix(".log").open("w") as log:
                    subprocess.run(command, check=True, stdout=log, stderr=subprocess.STDOUT)
                data = json.loads(dest.read_text())
                run = data["results"]["chromium"]["runs"][0]
                ctr = run["counters"]
                expected = {"game.wasm": identity["wasm"], "manifest.aetext": identity["runtimeManifest"]}
                if run["receipts"] != expected or run["frames"] != 600 or run["intervals"] != 600:
                    raise ValueError(f"{dest}: artifact or timing window mismatch")
                if expect.lower() not in run["renderer"].lower() or run["browserState"]["hidden"]:
                    raise ValueError(f"{dest}: unexpected GPU or hidden tab")
                if ctr["postFXFailures"] or (ctr["postFXPasses"] != 0) != enabled or (not enabled and ctr["postFXTargets"]):
                    raise ValueError(f"{dest}: stack did not take the requested path")
                pair[name] = {key: run[key] for key in ("fps","frameMs","workMs","gpuMs","counters","bridge","renderer","browserState","moduleSamples")}
                pair[name]["browserVersion"] = data["meta"]["browsers"]["chromium"]
            for key in ["moduleSamples", "renderer", "browserVersion", "browserState"]:
                if pair["on"][key] != pair["off"][key]:
                    raise ValueError(f"pair {i+1}: changed {key}")
            pair["deltaFPS"] = pair["on"]["fps"] - pair["off"]["fps"]
            report_file = out / "results.json"
            pending = out / "results.tmp"
            pending.write_text(json.dumps(report, indent=2) + "\n")
            pending.replace(report_file)
        if fingerprint(root) != identity:
            raise ValueError("export changed during measurement")
        report["medianPairedDeltaFPS"] = statistics.median(p["deltaFPS"] for p in report["pairs"])
        report["medianOfRuns"] = {
            mode: {"fps": statistics.median(p[mode]["fps"] for p in report["pairs"]),
                   "intervalMs": {k: statistics.median(p[mode]["frameMs"][k] for p in report["pairs"]) for k in ["p50","p95","p99"]}}
            for mode in ["on","off"]}
        report["complete"] = True
    finally:
        report["endUTC"] = datetime.now(timezone.utc).isoformat()
        report["loadAfter"] = os.getloadavg() if hasattr(os, "getloadavg") else None
        (out / "results.json").write_text(json.dumps(report, indent=2) + "\n")


if __name__ == "__main__":
    main()
