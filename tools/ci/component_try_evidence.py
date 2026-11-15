#!/usr/bin/env python3
"""Verify real Try observations, pixels and source identity without replaying them."""

import argparse
import base64
from dataclasses import dataclass
import gzip
import hashlib
import json
import math
from pathlib import Path
import re
import subprocess
import sys

from worktree_manifest import inventory

ENVIRONMENTS = ("native", "chromium", "firefox")
MAX_JSON = 64 * 1024 * 1024
MAX_PIXELS = 4 * 1024 * 1024
MAX_EXPANDED = 4 * 1024 * 1024 * 1024


class InvalidEvidence(ValueError):
    pass


@dataclass(frozen=True)
class Number:
    # Preserve json.Number's exact representation when tolerance is zero.
    text: str


def require(condition, message):
    if not condition:
        raise InvalidEvidence(message)


def object_pairs(pairs):
    out = {}
    for key, value in pairs:
        require(key not in out, "duplicate JSON key")
        out[key] = value
    return out


def load_json(path, compressed=False):
    path = Path(path)
    require(path.is_file() and not path.is_symlink(), f"missing or non-regular evidence: {path}")
    opener = gzip.open if compressed else open
    with opener(path, "rb") as stream:
        raw = stream.read(MAX_JSON + 1)
    require(len(raw) <= MAX_JSON, f"evidence exceeds decompression budget: {path}")

    def invalid_number(_):
        raise InvalidEvidence("non-finite JSON number")

    return json.loads(raw, parse_int=Number, parse_float=Number, parse_constant=invalid_number, object_pairs_hook=object_pairs), len(raw)


def field(obj, name, default=None):
    require(isinstance(obj, dict), "expected JSON object")
    names = list(dict.fromkeys(key for key in (name, name[:1].upper() + name[1:]) if key in obj))
    require(len(names) <= 1, f"ambiguous case aliases for {name}")
    return obj[names[0]] if names else default


def number(value, name):
    if isinstance(value, Number):
        value = float(value.text)
    require(type(value) in (int, float) and math.isfinite(value), f"invalid number: {name}")
    return value


def integer(value, name):
    if isinstance(value, Number):
        require(re.fullmatch(r"-?(0|[1-9][0-9]*)", value.text) is not None, f"expected integer: {name}")
        value = int(value.text)
    require(type(value) is int, f"expected integer: {name}")
    return value


def identities(values, name, nullable=False):
    if values is None and nullable:
        return set()
    require(isinstance(values, list), f"missing {name}")
    require(all(isinstance(x, str) and x.strip() for x in values), f"empty/non-string {name}")
    require(len(values) == len(set(values)), f"duplicate {name}")
    return set(values)


def matches(a, b, tolerance):
    if tolerance and isinstance(a, Number) and isinstance(b, Number):
        return abs(number(a, "observation") - number(b, "expectation")) <= tolerance
    return a == b


def source_snapshot(root, output, allow_unborn=False):
    root, output = Path(root).resolve(strict=True), Path(output).resolve()
    if output == root or root in output.parents:
        relative = output.relative_to(root).as_posix()
        require(subprocess.run(["git", "-C", str(root), "check-ignore", "-q", "--", relative]).returncode == 0,
                "snapshot inside checkout must be ignored")
    data = inventory(root, allow_unborn=allow_unborn)
    output.parent.mkdir(parents=True, exist_ok=True)
    with output.open("x", encoding="utf-8") as stream:
        json.dump(data, stream, indent=2, ensure_ascii=True)
        stream.write("\n")
    return data


def validate_snapshot(path):
    path = Path(path)
    require(path.is_file() and not path.is_symlink(), "missing or linked source snapshot")
    data = json.loads(path.read_bytes(), object_pairs_hook=object_pairs)
    require(data.get("schema") == 1 and isinstance(data.get("files"), list), "invalid source snapshot")
    records = data["files"]
    names = [row.get("path") for row in records]
    require(all(isinstance(name, str) for name in names) and names == sorted(set(names)), "invalid snapshot inventory")
    canonical = json.dumps(records, ensure_ascii=True, sort_keys=True, separators=(",", ":")).encode("ascii")
    require(hashlib.sha256(canonical).hexdigest() == data.get("worktreeSHA256"), "source snapshot hash mismatch")
    require(data.get("fileCount") == len(records), "source snapshot file count mismatch")
    return data


def pixels(frame):
    w, h = integer(field(frame, "width"), "frame width"), integer(field(frame, "height"), "frame height")
    require(0 < w <= 4096 and 0 < h <= 4096 and w * h <= MAX_PIXELS, "invalid frame geometry")
    encoded = field(frame, "pixels")
    require(isinstance(encoded, str) and len(encoded) <= ((w * h * 4 + 2) // 3) * 4, "missing or oversized pixels")
    try:
        rgba = base64.b64decode(encoded, validate=True)
    except ValueError as exc:
        raise InvalidEvidence("invalid base64 RGBA frame") from exc
    require(len(rgba) == w * h * 4, "Game View frame byte length mismatch")
    return w, h, rgba


def changed_pixels(before, after):
    aw, ah, a = pixels(before)
    bw, bh, b = pixels(after)
    require((aw, ah) == (bw, bh), "Game View frame geometry changed")
    return sum(x != y for x, y in zip(memoryview(a).cast("I"), memoryview(b).cast("I")))


def validate_observation(observation, contract, environment):
    require(field(observation, "identity") == contract["identity"], "stale or different Try payload")
    require(contract["componentID"] in identities(field(observation, "components"), "observed components"), "component is absent")
    deps = identities(field(observation, "dependencies", []), "observed dependencies", nullable=True)
    require(identities(contract.get("dependencies"), "contract dependencies", nullable=True).issubset(deps), "unresolved dependency")
    mode = contract["environments"][environment]["mode"]
    executing = field(observation, "runtimeExecuting")
    require(type(executing) is bool and executing == (mode == "runtime"), "runtime capability misrepresented")
    require(isinstance(field(observation, "observables"), dict), "missing observed state")


def verify(root, report_path, evidence, wasm_path, allow_unborn=False, during_gate=False):
    evidence = Path(evidence).resolve(strict=True)
    require(not Path(report_path).is_symlink(), "report must be a regular file")
    gate_status = Path(str(report_path) + ".exit").read_text().strip()
    if during_gate:
        require(gate_status == "RUNNING" and Path(str(report_path) + ".test.exit").read_text().strip() == "0",
                "component suite is not running or its exit code is nonzero")
    else:
        require(gate_status == "0", "component gate exit code is absent or nonzero")
    before = validate_snapshot(evidence / "source-before.json")
    after = validate_snapshot(evidence / "source-after.json")
    require(before == after, "source tree changed during the component suite")
    require(before == inventory(root, allow_unborn=allow_unborn), "observations do not match the current source tree")
    registry, _ = load_json(evidence / "registry.json")
    report, _ = load_json(report_path)
    versions, _ = load_json(evidence / "versions.json")
    require(isinstance(registry.get("goVersion"), str) and registry["goVersion"].startswith("go"), "Go version missing")
    require(all(isinstance(versions.get(key), str) and versions[key] for key in ("node", "playwright")), "browser tooling versions missing")
    require(all(isinstance(versions.get("browsers", {}).get(name), str) and versions["browsers"][name] for name in ("chromium", "firefox")), "browser versions missing")
    require(all(isinstance(versions.get("renderers", {}).get(name), dict) and
                all(isinstance(versions["renderers"][name].get(key), str) and versions["renderers"][name][key] for key in ("version", "renderer"))
                for name in ("chromium", "firefox")), "actual WebGL renderer details missing")
    require(integer(registry.get("schema"), "registry schema") == 1, "unsupported registry schema")
    run_id = registry.get("runID")
    require(isinstance(run_id, str) and run_id, "missing run identity")
    wasm = Path(wasm_path)
    require(wasm.is_file() and not wasm.is_symlink(), "missing tested WASM")
    wasm_sha = hashlib.sha256(wasm.read_bytes()).hexdigest()
    require(wasm_sha == registry.get("wasmSHA256"), "tested WASM differs from current artifact")
    components, contracts = registry.get("components"), registry.get("contracts")
    require(isinstance(components, list) and components and isinstance(contracts, list), "empty registry/contracts")
    component_ids = identities([c.get("ID") for c in components], "registry ComponentIDs")
    contract_ids = identities([c.get("componentID") for c in contracts], "contract ComponentIDs")
    require(component_ids == contract_ids, "contracts do not cover the public registry exactly")
    identities([c.get("demoID") for c in contracts], "DemoIDs")
    require(not field(report, "errors", []), "runtime report contains registry errors")
    require(integer(field(report, "publicComponents"), "public count") == len(components), "report public count mismatch")
    require(integer(field(report, "verifiedComponents"), "verified count") == len(components), "not every component was verified")
    results = field(report, "results")
    require(isinstance(results, list) and identities([field(r, "componentID") for r in results], "report ComponentIDs") == component_ids, "report entries differ from registry")
    by_id = {c["ID"]: c for c in components}
    report_ids = {field(r, "componentID"): r for r in results}
    count, frame_pairs, expanded = 0, 0, 0
    observations_used, summaries = set(), []
    for contract in contracts:
        cid, demo = contract["componentID"], contract["demoID"]
        require(re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._-]{0,159}", demo) is not None, "unsafe DemoID evidence path")
        component = by_id[cid]
        require(integer(contract.get("version"), "contract version") == 1, "unsupported contract version")
        for a, b in (("identity", "Identity"), ("documentID", "DocumentID"), ("scene", "Scene")):
            require(contract.get(a) and contract.get(a) == component.get(b), "contract not bound to registry")
        editable = identities(contract.get("editableFields"), "editable fields", nullable=True)
        require(editable.issubset(identities(component.get("Fields"), "schema fields", nullable=True)), "editable property absent")
        for lang in ("en", "ru"):
            text = contract.get("languages", {}).get(lang, {})
            require(all(isinstance(text.get(k), str) and text[k].strip() for k in ("title", "description", "input")), f"{cid}: missing {lang} instructions")
        require(contract.get("reset"), "missing reset behavior")
        actions = contract.get("actions")
        require(isinstance(actions, list) and 1 <= len(actions) <= 256, "invalid action count")
        identities([a.get("id") for a in actions], "action IDs")
        modified = set()
        warmup = integer(contract.get("warmupFrames", 0), "warmup frames")
        require(0 <= warmup <= 600, "warmup exceeds frame budget")
        for action in actions:
            target, toggle = action.get("field"), action.get("scriptToggle")
            keys = identities(action.get("keys"), "action keys", nullable=True)
            frames = integer(action.get("frames", 0), "action frames")
            inputs = action.get("inputs") or []
            require(0 <= frames <= 600 and len(keys) <= 8 and isinstance(inputs, list) and len(inputs) <= 8, "action exceeds input/frame budget")
            require(target or toggle or keys or inputs, "action contains no supported operation")
            if toggle is not None:
                require(isinstance(toggle, dict) and not target and "value" not in action and type(toggle.get("enabled")) is bool and isinstance(toggle.get("script"), str) and toggle["script"] == toggle["script"].strip() and 0 < len(toggle["script"]) <= 256, "invalid script attachment action")
            if target:
                require(target in editable and "value" in action, "action does not edit a declared property")
                modified.add(target)
            elif not toggle:
                require("value" not in action, "input-only action has an unexpected value")
            if frames == 0 and keys:
                frames = warmup or 4
            for step in inputs:
                count_frames = integer(step.get("frames"), "input frames")
                frames += count_frames
                require(1 <= count_frames <= 600 and frames <= 600 and len(identities(step.get("keys"), "input keys", nullable=True)) <= 8, "input sequence exceeds frame/key budget")
        require(modified == editable, "an editable field has no tested action")
        result = report_ids[cid]
        require(field(result, "demoID") == demo and not field(result, "errors", []), "demo runtime failed or has a different ID")
        require(set(ENVIRONMENTS).issubset(identities(field(result, "verifiedEnvironments"), "report environments")), "native, Chromium and Firefox must all run")
        for environment in ENVIRONMENTS:
            compat = contract.get("environments", {}).get(environment)
            require(isinstance(compat, dict) and compat.get("mode") in ("runtime", "authoring"), "missing environment capability")
            if compat["mode"] == "authoring":
                require(all(compat.get("reason", {}).get(lang) for lang in ("en", "ru")), "authoring limitation is not localized")
            directory = evidence / environment / demo
            require(directory.is_dir() and not directory.is_symlink() and not directory.parent.is_symlink(), "missing or linked observation directory")
            require(len(list(directory.glob("observation-*.json.gz"))) == 3 * len(actions), "expected before/after/reset for every action")
            for index, action in enumerate(actions):
                triple = []
                for phase in range(3):
                    file = directory / f"observation-{3 * index + phase:03d}.json.gz"
                    wrapper, size = load_json(file, compressed=True)
                    expanded += size
                    require(expanded <= MAX_EXPANDED, "aggregate decompressed evidence budget exceeded")
                    require(integer(wrapper.get("schema"), "observation schema") == 1 and wrapper.get("runID") == run_id and wrapper.get("environment") == environment and wrapper.get("demoID") == demo, "mixed run/environment/demo observations")
                    observation = wrapper.get("observation")
                    validate_observation(observation, contract, environment)
                    triple.append(observation)
                    observations_used.add(file)
                    count += 1
                expected = action.get("expected")
                require(isinstance(expected, list) and expected and any(e.get("changes") is True for e in expected), "action lacks observable change")
                identities([e.get("observable") for e in expected], "expected observables")
                for expectation in expected:
                    key = expectation["observable"]
                    tolerance = number(expectation.get("tolerance", 0), "tolerance")
                    require(tolerance >= 0 and "equals" in expectation, "invalid expectation")
                    states = [field(o, "observables") for o in triple]
                    require(all(key in state for state in states), "observable missing before/action/reset")
                    require(matches(states[1][key], expectation["equals"], tolerance), f"{environment}/{demo}/{action['id']}: unexpected observable {key}")
                    require(not expectation.get("changes") or not matches(states[0][key], states[1][key], tolerance), "observable did not change")
                    require(matches(states[0][key], states[2][key], tolerance), "Reset did not restore baseline")
                visual = action.get("visual")
                if visual is not None:
                    minimum = integer(visual.get("minChangedPixels"), "changed pixel minimum")
                    maximum = integer(visual.get("maxResetDifferencePixels"), "reset pixel maximum")
                    require(minimum > 0 and maximum >= 0, "invalid visual threshold")
                    frames = [field(o, "frames", []) for o in triple]
                    expected_frames = 1 if environment == "native" else 2
                    require(all(isinstance(f, list) and len(f) == expected_frames for f in frames), "native or EN/RU browser Game View frames missing")
                    for language in range(expected_frames):
                        delta = changed_pixels(frames[0][language], frames[1][language])
                        restored = changed_pixels(frames[0][language], frames[2][language])
                        require(delta >= minimum and restored <= maximum, f"{environment}/{demo}: RGBA action/reset failed ({delta}/{restored})")
                        frame_pairs += 2
                        summaries.append({"componentID": cid, "environment": environment, "action": action["id"], "language": "native" if environment == "native" else ("en", "ru")[language], "changedPixels": delta, "resetDifferencePixels": restored})
    require(set(evidence.glob("*/**/observation-*.json.gz")) == observations_used, "unexpected or stale observations")
    return {"schema": 1, "result": "SUITE_VERIFIED" if during_gate else "PASS", "runID": run_id, "sourceFingerprint": before["worktreeSHA256"], "head": before["head"], "wasmSHA256": wasm_sha, "goVersion": registry["goVersion"], "browserVersions": versions, "publicComponents": len(components), "verifiedComponents": len(components), "environments": list(ENVIRONMENTS), "observations": count, "rawRGBAFrameComparisons": frame_pairs, "decompressedBytes": expanded, "visualResults": summaries}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    modes = parser.add_subparsers(dest="mode", required=True)
    snapshot = modes.add_parser("snapshot")
    snapshot.add_argument("--root", "--source", required=True, type=Path)
    snapshot.add_argument("--output", required=True, type=Path)
    snapshot.add_argument("--allow-unborn", action="store_true")
    check = modes.add_parser("verify")
    check.add_argument("--root", "--source", required=True, type=Path)
    check.add_argument("--report", required=True, type=Path)
    check.add_argument("--evidence", required=True, type=Path)
    check.add_argument("--wasm", required=True, type=Path)
    check.add_argument("--output", type=Path)
    check.add_argument("--allow-unborn", action="store_true")
    check.add_argument("--during-gate", action="store_true", help="internal: validate successful suite while the enclosing gate is still running")
    opt = parser.parse_args()
    try:
        if opt.mode == "snapshot":
            result = source_snapshot(opt.root, opt.output, opt.allow_unborn)
            print(json.dumps({"head": result["head"], "sourceFingerprint": result["worktreeSHA256"], "files": result["fileCount"]}))
        else:
            result = verify(opt.root, opt.report, opt.evidence, opt.wasm, opt.allow_unborn, opt.during_gate)
            if opt.output:
                opt.output.parent.mkdir(parents=True, exist_ok=True)
                with opt.output.open("x", encoding="utf-8") as stream:
                    json.dump(result, stream, indent=2)
                    stream.write("\n")
            print(json.dumps({key: value for key, value in result.items() if key != "visualResults"}))
    except (OSError, ValueError, KeyError, TypeError, RuntimeError, subprocess.CalledProcessError) as exc:
        print("component Try evidence: " + str(exc), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
