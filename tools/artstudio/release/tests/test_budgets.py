import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


class StandaloneDependencyBudgetTest(unittest.TestCase):
    def run_budget(self, extra_dependency):
        with tempfile.TemporaryDirectory() as tmp:
            artifact = Path(tmp)
            manifest = {
                'wasm': {'path': 'assets/core.wasm', 'bytes': 100},
                'previewWasm': {'path': 'assets/preview.wasm', 'bytes': 200},
                'files': [
                    {'path': 'assets/core.wasm.gz', 'bytes': 50},
                    {'path': 'assets/preview.wasm.gz', 'bytes': 100},
                ],
            }
            (artifact / 'manifest.json').write_text(json.dumps(manifest))
            (artifact / 'dependency-audit.txt').write_text(
                'fmt\naego/art\naego/tools/artstudio/host\n'
                'aego/tools/artstudio/wasm\n' + extra_dependency + '\n'
            )
            (artifact / 'preview-dependency-audit.txt').write_text('aego/art\n')
            return subprocess.run(
                [sys.executable, str(Path(__file__).with_name('budgets.py')),
                 '--artifact', str(artifact)],
                capture_output=True, text=True, check=False,
            )

    def test_only_the_explicit_internal_codec_is_allowed(self):
        result = self.run_budget('aego/art/internal/aseprite')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout)['standaloneBudget'], 'PASS')
        for forbidden in ('aego/art/internal', 'aego/art/internal/other',
                          'aego/art/internal/aseprite/other', 'aego/editor/shell'):
            with self.subTest(dependency=forbidden):
                result = self.run_budget(forbidden)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn('unexpected engine dependency: ' + forbidden, result.stderr)


if __name__ == '__main__':
    unittest.main()
