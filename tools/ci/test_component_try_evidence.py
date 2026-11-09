import base64
import gzip
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile
import unittest

from component_try_evidence import InvalidEvidence, Number, matches, source_snapshot, verify
from worktree_manifest import inventory


class ComponentEvidenceTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        subprocess.run(["git", "init", "-q", str(self.root)], check=True)
        (self.root / ".gitignore").write_text(".cache/\n")
        (self.root / "source.go").write_text("package example\n")
        self.evidence = self.root / ".cache/evidence"
        self.evidence.mkdir(parents=True)
        self.wasm = self.root / ".cache/studio.wasm"
        self.wasm.write_bytes(b"fixture artifact; tests do not execute it")
        self.report = self.root / ".cache/report.json"
        self.write_json(self.report, {"publicComponents": 1, "verifiedComponents": 1, "results": [{"componentID": "render.Sprite", "demoID": "sprite", "verifiedEnvironments": ["native", "chromium", "firefox"]}]})
        Path(str(self.report) + ".exit").write_text("0\n")
        self.contract = {"version": 1, "componentID": "render.Sprite", "demoID": "sprite", "identity": "identity", "documentID": "render/Sprite", "scene": "sprite.aescene", "dependencies": None, "editableFields": ["Opacity"], "reset": "reset", "languages": {lang: {"title": "Sprite", "description": "Change opacity", "input": "Move slider"} for lang in ("en", "ru")}, "environments": {env: {"mode": "runtime"} for env in ("native", "chromium", "firefox")}, "actions": [{"id": "opacity", "field": "Opacity", "value": 1, "expected": [{"observable": "opacity", "equals": 1, "changes": True}], "visual": {"minChangedPixels": 1, "maxResetDifferencePixels": 0}}]}
        self.registry = {"schema": 1, "runID": "test-run", "wasmSHA256": hashlib.sha256(self.wasm.read_bytes()).hexdigest(), "goVersion": "go1.27.0", "components": [{"ID": "render.Sprite", "Identity": "identity", "DocumentID": "render/Sprite", "Scene": "sprite.aescene", "Fields": ["Opacity"]}], "contracts": [self.contract]}
        self.write_json(self.evidence / "registry.json", self.registry)
        self.versions = {"node": "v1", "playwright": "1", "browsers": {"chromium": "1", "firefox": "1"}, "renderers": {name: {"version": "WebGL 2", "renderer": "fixture"} for name in ("chromium", "firefox")}}
        self.write_json(self.evidence / "versions.json", self.versions)
        for env in ("native", "chromium", "firefox"):
            for index, value in enumerate((0, 1, 0)):
                observation = {"identity": "identity", "components": ["render.Sprite"], "dependencies": None, "runtimeExecuting": True, "observables": {"opacity": value}, "frames": [{"width": 1, "height": 1, "pixels": base64.b64encode(bytes([value, 0, 0, 255])).decode()} for _ in range(1 if env == "native" else 2)]}
                if env == "native":
                    observation = {(key[0].upper() + key[1:] if key != "frames" else key): value for key, value in observation.items()}
                self.write_observation(env, index, {"schema": 1, "runID": "test-run", "environment": env, "demoID": "sprite", "observation": observation})
        source_snapshot(self.root, self.evidence / "source-before.json", True)
        source_snapshot(self.root, self.evidence / "source-after.json", True)

    @staticmethod
    def write_json(path, data):
        path.write_text(json.dumps(data))

    def observation_path(self, env, index):
        return self.evidence / env / "sprite" / f"observation-{index:03d}.json.gz"

    def write_observation(self, env, index, data):
        path = self.observation_path(env, index)
        path.parent.mkdir(parents=True, exist_ok=True)
        with gzip.open(path, "wt") as stream:
            json.dump(data, stream)

    def mutate_observation(self, env, index, mutation):
        with gzip.open(self.observation_path(env, index), "rt") as stream:
            wrapper = json.load(stream)
        mutation(wrapper)
        self.write_observation(env, index, wrapper)

    def verify(self):
        return verify(self.root, self.report, self.evidence, self.wasm, True)

    def test_complete_evidence_rechecks_pixels_observations_and_source(self):
        result = self.verify()
        self.assertEqual(result["result"], "PASS")
        self.assertIsNone(result["head"])
        self.assertEqual(result["observations"], 9)
        self.assertEqual(result["rawRGBAFrameComparisons"], 10)
        self.assertEqual(len(result["visualResults"]), 5)

    def test_unborn_requires_explicit_opt_in(self):
        with self.assertRaises(subprocess.CalledProcessError):
            inventory(self.root)
        self.assertIsNone(inventory(self.root, allow_unborn=True)["head"])

    def test_snapshot_does_not_overwrite_or_pollute_source(self):
        with self.assertRaises(FileExistsError):
            source_snapshot(self.root, self.evidence / "source-before.json", True)
        with self.assertRaises(InvalidEvidence):
            source_snapshot(self.root, self.root / "inventory.json", True)

    def test_source_mutation_invalidates_saved_evidence(self):
        (self.root / "source.go").write_text("package changed\n")
        with self.assertRaisesRegex(InvalidEvidence, "current source"):
            self.verify()

    def test_wasm_changed_after_run_is_rejected(self):
        self.wasm.write_bytes(b"different")
        with self.assertRaisesRegex(InvalidEvidence, "WASM differs"):
            self.verify()

    def test_missing_browser_renderer_and_nonzero_exit_are_rejected(self):
        report = json.loads(self.report.read_text())
        report["results"][0]["verifiedEnvironments"].remove("firefox")
        self.write_json(self.report, report)
        with self.assertRaisesRegex(InvalidEvidence, "must all run"):
            self.verify()
        Path(str(self.report) + ".exit").write_text("1\n")
        with self.assertRaisesRegex(InvalidEvidence, "exit code"):
            self.verify()
        Path(str(self.report) + ".exit").write_text("0\n")
        report["results"][0]["verifiedEnvironments"].append("firefox")
        self.write_json(self.report, report)
        del self.versions["renderers"]["firefox"]
        self.write_json(self.evidence / "versions.json", self.versions)
        with self.assertRaisesRegex(InvalidEvidence, "renderer details missing"):
            self.verify()

    def test_all_three_observations_are_required(self):
        self.observation_path("firefox", 2).unlink()
        with self.assertRaisesRegex(InvalidEvidence, "before/after/reset"):
            self.verify()

    def test_running_gate_cannot_be_mistaken_for_a_completed_pass(self):
        Path(str(self.report) + ".exit").write_text("RUNNING\n")
        Path(str(self.report) + ".test.exit").write_text("0\n")
        with self.assertRaisesRegex(InvalidEvidence, "exit code"):
            self.verify()
        self.assertEqual(verify(self.root, self.report, self.evidence, self.wasm, True, True)["result"], "SUITE_VERIFIED")
        Path(str(self.report) + ".test.exit").write_text("1\n")
        with self.assertRaisesRegex(InvalidEvidence, "suite"):
            verify(self.root, self.report, self.evidence, self.wasm, True, True)

    def test_actual_pixels_must_change_and_reset(self):
        self.mutate_observation("chromium", 1, lambda w: w["observation"]["frames"][0].update(pixels=base64.b64encode(bytes([0, 0, 0, 255])).decode()))
        with self.assertRaisesRegex(InvalidEvidence, "RGBA action/reset"):
            self.verify()
        self.mutate_observation("chromium", 1, lambda w: w["observation"]["frames"][0].update(pixels=base64.b64encode(bytes([1, 0, 0, 255])).decode()))
        self.mutate_observation("firefox", 2, lambda w: w["observation"]["frames"][1].update(pixels=base64.b64encode(bytes([1, 0, 0, 255])).decode()))
        with self.assertRaisesRegex(InvalidEvidence, "RGBA action/reset"):
            self.verify()

    def test_observable_reset_and_identity_are_checked(self):
        self.mutate_observation("native", 2, lambda w: w["observation"]["Observables"].update(opacity=1))
        with self.assertRaisesRegex(InvalidEvidence, "Reset did not restore"):
            self.verify()
        self.mutate_observation("native", 2, lambda w: w["observation"].update(Identity="stale"))
        with self.assertRaisesRegex(InvalidEvidence, "stale or different"):
            self.verify()

    def test_stale_run_and_extra_files_are_rejected(self):
        self.mutate_observation("firefox", 0, lambda w: w.update(runID="old-run"))
        with self.assertRaisesRegex(InvalidEvidence, "mixed run"):
            self.verify()
        self.mutate_observation("firefox", 0, lambda w: w.update(runID="test-run"))
        extra = self.evidence / "native/old-demo/observation-000.json.gz"
        extra.parent.mkdir()
        extra.write_bytes(b"stale")
        with self.assertRaisesRegex(InvalidEvidence, "unexpected or stale"):
            self.verify()

    def test_overbudget_frames_are_rejected(self):
        self.mutate_observation("firefox", 0, lambda w: w["observation"]["frames"][0].update(width=999999999))
        with self.assertRaisesRegex(InvalidEvidence, "geometry"):
            self.verify()

    def test_json_numeric_matching_is_not_python_coercion(self):
        self.assertFalse(matches(Number("1"), True, 0))
        self.assertFalse(matches(Number("1"), Number("1.0"), 0))
        self.assertTrue(matches(Number("1"), Number("1.0001"), 0.001))
        self.assertFalse(matches("1", Number("1"), 0.001))


if __name__ == "__main__":
    unittest.main()
