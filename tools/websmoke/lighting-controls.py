"""Build disposable scene-data controls for residual PostFX-OFF profiling."""
import argparse
import json
from pathlib import Path
import re
import shutil
import subprocess

from particles import fingerprint


def changed(text, old, new, count=1):
    if text.count(old) != count:
        raise ValueError(f"canonical project changed near {old!r}")
    return text.replace(old, new)


def configure(project, mode):
    if mode == "production":
        return
    if mode == "particles-off":
        cfg = project / "project.aego"
        text = cfg.read_text()
        if "particles.enabled" in text or re.search(r"\bparticles\s*\{", text):
            raise ValueError("particle project default already exists")
        cfg.write_text(changed(text, "defaults {", "defaults {\n    particles {\n      enabled=false\n    }"))
        return
    scene = project / "scenes/main.aescene"
    text = changed(scene.read_text(), "maxShadowedLights=3", "maxShadowedLights=0")
    if mode == "unlit-hdr":
        text = changed(text, "colorMode=auto", "colorMode=hdr")
    scene.write_text(text)
    if mode == "masks-off":
        return

    # Keep the world, lamp and character material IDs and numeric bindings,
    # including Flash. Only these temporary copies share the unlit program.
    mats = project / "assets/materials"
    for name in ["world_lit", "lamp_lit"]:
        file = mats / (name + ".aemat")
        file.write_text(changed(file.read_text(), "ae900000000000000000000000000c86", "af120e00000000000000000000000001"))
    shader = mats / "character_flash_lit.aeshader"
    text = changed(shader.read_text(), "#pragma lighting forward2d\n", "")
    text = changed(text, '#include "engine/lighting.glsl"\n', "")
    text, n = re.subn(r"^uniform sampler2D (NormalMap|EmissionMap);.*\n", "", text, flags=re.M)
    if n != 2:
        raise ValueError("unexpected character material inputs")
    text = changed(text, "vec4 lit = aego_lit(aego_sample(), NormalStrength,\n        aego_emission(EmissionColor.rgb, EmissionIntensity), ReceiveShadows);",
                         "vec4 lit = aego_sample();")
    shader.write_text(text)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("engine", type=Path)
    parser.add_argument("cli", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    engine, cli, out = (p.resolve() for p in [args.engine, args.cli, args.output])
    if engine == out or engine in out.parents:
        parser.error("use a new output directory outside the source checkout")
    out.mkdir(parents=True, exist_ok=False)
    report = {}
    for mode in ["production", "particles-off", "masks-off", "unlit-hdr", "unlit-direct"]:
        source = out / "projects" / mode
        shutil.copytree(engine / "examples/platformer", source,
                        ignore=shutil.ignore_patterns(".git", ".cache", ".aego", "build"))
        module_file = source / "go.mod"
        module_file.write_text(changed(module_file.read_text(), "replace aego => ../..", "replace aego => " + json.dumps(str(engine))))
        configure(source, mode)
        export = out / "exports" / mode
        with (out / (mode + ".log")).open("w") as log:
            subprocess.run([str(cli), "export", str(source), "--engine", str(engine), "--target", "web", "--release", "--out", str(export)],
                           cwd=engine, stdout=log, stderr=subprocess.STDOUT, check=True)
        report[mode] = fingerprint(export)
        (out / "controls.json").write_text(json.dumps(report, indent=2) + "\n")
        if report[mode]["wasm"] != report["production"]["wasm"]:
            raise ValueError("the exports contain different game.wasm binaries; do not compare them")


if __name__ == "__main__":
    main()
