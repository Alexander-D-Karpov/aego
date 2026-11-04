import hashlib
import json
from pathlib import Path
import tempfile
import unittest

from particles import fingerprint, window_births


class ParticleComparison(unittest.TestCase):
    def test_reject_same_size_artifact_change(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            art = root / "data/artifacts"
            art.mkdir(parents=True)
            scene = b"prepared scene"
            name = hashlib.sha256(scene).hexdigest()[:24] + ".scene"
            (art / name).write_bytes(scene)
            manifest = f'  path="data://artifacts/{name}"\n'.encode()
            (root / "data/manifest.aetext").write_bytes(manifest)
            (root / "game.wasm").write_bytes(b"wasm")
            content = hashlib.sha256(name.encode() + manifest).hexdigest()
            (root / "export.aetext").write_text(f'game="Platformer"\ntarget="web"\nprofile="release"\nhash="{content}"\n')
            (root / "data/files.json").write_text(json.dumps({
                "wasm": 4, "manifest": {"path": "manifest.aetext", "size": len(manifest)},
                "files": [{"path": "artifacts/" + name, "size": len(scene)}]}))
            self.assertEqual(fingerprint(root)["exportContent"], content)
            (art / name).write_bytes(b"X" + scene[1:])
            with self.assertRaisesRegex(ValueError, "hash mismatch"):
                fingerprint(root)

    def test_an_enabled_but_idle_module_is_not_the_on_control(self):
        def samples(a, b):
            return [{"particles.spawned": n} for n in (a, b)]
        self.assertIsNone(window_births("A", None))
        self.assertEqual(window_births("C", samples(8, 16)), 8)
        self.assertEqual(window_births("B", samples(0, 0)), 0)
        for mode, counters in (("C", samples(8, 8)), ("B", samples(8, 8)), ("C", None)):
            with self.assertRaises(ValueError):
                window_births(mode, counters)
