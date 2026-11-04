"""Compare existing platformer exports. No build, deploy, or golden updates."""
import argparse
import hashlib
import json
from pathlib import Path
import platform
import re
import shutil
import statistics
import subprocess
import sys
import tempfile

HERE = Path(__file__).resolve().parent
ROUTE = HERE.parents[1] / "tests/integration/testdata/platformer_lighting_route.aeinput"
ROUTE_SHA = "c1ad101708d3674878c7ba309ddc8f2c3ff89f645e77f545c0851e162bcd3415"
A_WASM = "5e4a1ddd586ae67db0fd761b0893644eb582b1b6a74769c9d46a3a7958387e2a"
A_MANIFEST = "1a2ad0d7c4a6ec65dacf8233be05726ad61e77a5f6234d3d0ec0ca0103483747"


def sha(file):
    with file.open("rb") as f:
        h = hashlib.sha256()
        while chunk := f.read(1 << 20):
            h.update(chunk)
        return h.hexdigest()


def fingerprint(root):
    export = root / "export.aetext"
    text = export.read_text()
    def field(key):
        m = re.search(r'^\s*' + key + r'="([^"]+)"\s*$', text, re.M)
        if not m:
            raise ValueError(f"{export}: missing {key}")
        return m[1]

    if [field(k) for k in ("game", "target", "profile")] != ["Platformer", "web", "release"]:
        raise ValueError(f"{root} is not a platformer release web export")
    listing = json.loads((root / "data/files.json").read_text())
    if listing["manifest"]["path"] != "manifest.aetext":
        raise ValueError("unexpected runtime manifest path")
    names = []
    for entry in [listing["manifest"], *listing["files"]]:
        p = Path(entry["path"])
        if p.is_absolute() or ".." in p.parts or (root / "data" / p).stat().st_size != entry["size"]:
            raise ValueError(f"invalid export file: {p}")
    for entry in listing["files"]:
        rel = entry["path"]
        match = re.fullmatch(r"artifacts/([a-f0-9]{24})\.([a-zA-Z0-9_-]+)", rel)
        if not match or not sha(root / "data" / rel).startswith(match[1]):
            raise ValueError(f"prepared artifact hash mismatch: {rel}")
        names.append(Path(rel).name)
    manifest = (root / "data/manifest.aetext").read_bytes()
    declared = re.findall(rb'^\s*path="data://artifacts/([^"]+)"\s*$', manifest, re.M)
    if len(set(names)) != len(names) or set(names) != {name.decode() for name in declared}:
        raise ValueError("runtime manifest and files.json disagree on artifacts")
    content = hashlib.sha256("\n".join(sorted(names)).encode() + manifest).hexdigest()
    if content != field("hash"):
        raise ValueError("export content hash does not match its manifest and artifacts")
    wasm = root / "game.wasm"
    if wasm.stat().st_size != listing["wasm"]:
        raise ValueError("WASM size disagrees with export")
    h = hashlib.sha256()
    for file in sorted(root.rglob("*")):
        if file.is_symlink():
            raise ValueError(f"symlink in export: {file}")
        if file.is_file():
            h.update(file.relative_to(root).as_posix().encode() + b"\0" + bytes.fromhex(sha(file)))
    return {"tree": h.hexdigest(), "wasm": sha(wasm), "wasmBytes": listing["wasm"],
            "wasmGzipBytes": listing.get("wasmGzip"), "exportFile": sha(export), "exportContent": content,
            "runtimeManifest": sha(root / "data" / listing["manifest"]["path"]),
            "particles": bool(re.search(r'^\s*modules=\[[^\n]*"particles"', text, re.M))}


def check_a(root):
    a = fingerprint(root)
    identities = [k for k in ("exportFile", "exportContent", "runtimeManifest") if a[k] == A_MANIFEST]
    if a["wasm"] != A_WASM or not identities or a["particles"]:
        raise ValueError(f"export A does not match the reported 12b reference: {a}")
    a["matchedManifest"] = identities
    return a


def window_births(mode, samples):
    if mode == "A":
        return None
    if not isinstance(samples, list) or len(samples) != 2:
        raise ValueError(f"{mode}: runtime did not provide initial and final particle counters")
    start, end = (point.get("particles.spawned") if isinstance(point, dict) else None for point in samples)
    if not all(type(n) is int and n >= 0 for n in (start, end)):
        raise ValueError(f"{mode}: missing or invalid particle birth counters")
    if (mode == "B" and (start or end)) or (mode == "C" and end <= start):
        raise ValueError(f"{mode}: particle activity disagrees with session toggle ({start} -> {end})")
    return end - start


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="cmd", required=True)
    freeze = sub.add_parser("freeze", help="verify and copy the original CI export to a new directory")
    freeze.add_argument("source", type=Path)
    freeze.add_argument("destination", type=Path)
    run = sub.add_parser("run", help="measure five alternating pairs, using exports already built by CI")
    run.add_argument("reference", type=Path)
    run.add_argument("candidate", type=Path)
    run.add_argument("out", type=Path)
    run.add_argument("--gpu", choices=["soft", "hard"], default="soft")
    run.add_argument("--headed", action="store_true")
    run.add_argument("--expect-renderer", help="distinctive model string from the hardware reference, e.g. Iris")
    args = parser.parse_args()
    if sha(ROUTE) != ROUTE_SHA:
        raise ValueError("canonical input recording changed")
    if args.cmd == "freeze":
        src, dst = args.source.resolve(), args.destination.resolve()
        a = check_a(src)
        if dst.exists() or dst.is_relative_to(src):
            raise ValueError("destination must be new and outside the original export")
        dst.parent.mkdir(parents=True, exist_ok=True)
        with tempfile.TemporaryDirectory(dir=dst.parent) as temp:
            copy = Path(temp) / "export"
            shutil.copytree(src, copy, symlinks=True)
            if check_a(copy) != a:
                raise ValueError("source changed while it was copied")
            copy.rename(dst)
        print(json.dumps({"frozen": str(dst), "reference": a, "replay": ROUTE_SHA}, indent=2))
        return

    if args.gpu == "hard" and not args.expect_renderer:
        parser.error("--gpu hard requires --expect-renderer to identify the reference hardware")
    old, new, out = args.reference.resolve(), args.candidate.resolve(), args.out.resolve()
    a, bc = check_a(old), fingerprint(new)
    if not bc["particles"]:
        raise ValueError("candidate export has no particles module")
    if out.is_relative_to(old) or out.is_relative_to(new):
        raise ValueError("output must be outside both measured exports")
    out.mkdir(parents=True, exist_ok=False)
    report = dict(A=a, BC=bc, replay=ROUTE_SHA, seed=42, viewport=[1280, 720], dpr=1, lighting="production",
                  lightingDebug=False, warmup=120, intervals=600, host=platform.node(), platform=platform.platform(),
                  requestedGPU=args.gpu, renderer=None, browserVersion=None, runs=[], paired={})
    # A/B measure the idle module; B/C change only the session setting in the same artifact.
    for i in range(5):
        order = ("A", "B", "C") if i % 2 == 0 else ("C", "B", "A")
        for mode in order:
            dest = out / f"{i + 1}-{mode}.json"
            cmd = ["node", str(HERE / "bench.mjs"), f"--root={old if mode == 'A' else new}", f"--out={dest}",
                   "--md=false", "--measure-only=true", "--runs=1", "--browsers=chromium", "--warmup=120",
                   "--frames=600", "--profile=false", "--seed=42", f"--replay={ROUTE}", f"--gpu={args.gpu}",
                   "--exact-intervals=true", f"--headed={str(args.headed).lower()}"]
            if mode != "A":
                cmd.append("--bool=particles.enabled:" + ("1" if mode == "C" else "0"))
            with dest.with_suffix(".log").open("w") as log:
                subprocess.run(cmd, stdout=log, stderr=subprocess.STDOUT, check=True, cwd=HERE)
            data = json.loads(dest.read_text())
            measured = data["results"]["chromium"]["summary"]
            gaps = measured["samples"]["gaps"]
            if len(gaps) != 600 or min(gaps) <= 0:
                raise ValueError(f"invalid intervals in {dest}")
            renderer, version = measured["renderer"], data["meta"]["browsers"]["chromium"]
            if not renderer or renderer.lower() in ("webkit webgl", "webgl", "webgl 2.0", "angle", "unknown"):
                raise ValueError(f"browser did not identify the actual GPU: {renderer}")
            if args.expect_renderer and args.expect_renderer.lower() not in renderer.lower():
                raise ValueError(f"renderer differs from the requested reference: {renderer}")
            if (not renderer or ("swiftshader" in renderer.lower()) != (args.gpu == "soft")
                    or (args.gpu == "hard" and any(x in renderer.lower() for x in ("llvmpipe", "software", "lavapipe", "softpipe")))):
                raise ValueError(f"cannot verify requested renderer: {renderer}")
            if report["renderer"] not in (None, renderer) or report["browserVersion"] not in (None, version):
                raise ValueError("renderer or browser version changed between pairs")
            run_data = data["results"]["chromium"]["runs"][0]
            births = window_births(mode, run_data.get("moduleSamples"))
            report.update(renderer=renderer, browserVersion=version)
            report["runs"].append(dict(pair=i+1, mode=mode, fps=1000/statistics.mean(gaps),
                                       births=births, p50=measured["frameMs"]["p50"], p95=measured["frameMs"]["p95"],
                                       over50=sum(g>50 for g in gaps)/6, draws=measured["drawsPerFrame"],
                                       bufferKB=measured["bridge"].get("bufferKB")))
            print(f"pair {i+1}: {mode} {report['runs'][-1]['fps']:.2f} FPS", flush=True)
            (out / "results.json").write_text(json.dumps(report, indent=2) + "\n")
    if check_a(old) != a or fingerprint(new) != bc:
        raise ValueError("export changed during measurement")
    for prev, cur in [("A", "B"), ("B", "C")]:
        pairs = [{r["mode"]: r for r in report["runs"] if r["pair"] == i} for i in range(1, 6)]
        report["paired"][cur + "-" + prev] = {k: statistics.median(p[cur][k]-p[prev][k] for p in pairs)
                                                for k in ("fps", "p50", "p95", "over50", "draws")}
    (out / "results.json").write_text(json.dumps(report, indent=2) + "\n")


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, subprocess.CalledProcessError) as exc:
        sys.exit(str(exc))
