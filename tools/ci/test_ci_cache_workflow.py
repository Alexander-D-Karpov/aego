"""Check that optional cache failures never replace or suppress CI gates."""

from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = ROOT / '.github/workflows/ci.yml'


def job(name, next_name):
    text = WORKFLOW.read_text(encoding='utf-8')
    return text.split('\n  ' + name + ':\n', 1)[1].split('\n  ' + next_name + ':\n', 1)[0]


class CacheWorkflowTest(unittest.TestCase):
    def test_each_job_has_optional_bounded_cache_without_skipping_gates(self):
        jobs = (('checks', 'native-authoring', 'Asset Studio snapshot and real browser integration',
                 'Preserve full CI and component evidence'),
                ('native-authoring', 'build', 'Download the tested Studio snapshot', 'installer dry run'),
                ('build', 'release', 'build', 'example project'))
        for name, next_name, first_gate, last_gate in jobs:
            with self.subTest(job=name):
                body = job(name, next_name)
                self.assertEqual(body.count('name: Identify Go build cache'), 1)
                self.assertEqual(body.count('name: Restore Go build cache'), 1)
                self.assertEqual(body.count('name: Check optional Go cache upload'), 1)
                self.assertEqual(body.count('name: Save Go build cache'), 1)
                self.assertLess(body.index('bash tools/bootstrap-native.sh'), body.index('name: Identify Go build cache'))
                self.assertLess(body.index('name: Restore Go build cache'), body.index('name: ' + first_gate))
                self.assertLess(body.index('name: ' + last_gate), body.index('name: Check optional Go cache upload'))
                self.assertNotIn('uses: actions/setup-go@v5', body)
                self.assertIn('tools/ci/env/preflight.py', body)
                self.assertIn('uses: actions/cache/restore@v4', body)
                self.assertIn('uses: actions/cache/save@v4', body)
                self.assertEqual(body.count('key: aego-go-v1-${{ steps.go_cache_key.outputs.fingerprint }}-${{ github.sha }}'), 2)
                self.assertIn('aego-go-v1-${{ steps.go_cache_key.outputs.fingerprint }}-', body)
                self.assertIn("steps.go_cache_size.outputs.save == 'yes'", body)
                self.assertIn("steps.go_cache.outputs.cache-hit != 'true'", body)
                for step in ('Identify Go build cache', 'Restore Go build cache',
                             'Check optional Go cache upload', 'Save Go build cache'):
                    section = body.split('      - name: ' + step + '\n', 1)[1].split('\n      - ', 1)[0]
                    self.assertIn('continue-on-error: true', section)

    def test_github_native_keeps_every_platform_gate_and_bounds_optional_cache(self):
        text = (ROOT / '.github/workflows/github-native.yml').read_text(encoding='utf-8')
        body = text.split('\n  native:\n', 1)[1].split('\n  gitea-result:\n', 1)[0]
        for name in ('windows-2025', 'macos-15', 'macos-15-intel', 'ubuntu-24.04'):
            self.assertIn(name, body)
        self.assertLess(body.index('name: Build pinned SDL'), body.index('name: Identify Go build cache'))
        self.assertLess(body.index('name: Restore Go build cache'), body.index('name: Build all packages'))
        self.assertLess(body.index('name: Package Linux CLI'), body.index('name: Check optional Go cache upload'))
        for gate in ('Native platform tests', 'Linux compatibility tests', 'Strict headless smoke',
                     'CLI and headless runtime smoke', 'Test Windows installer bootstrap',
                     'Native silent installer and uninstaller', 'Build macOS app bundle'):
            self.assertIn('name: ' + gate, body)
        self.assertIn('uses: actions/cache/restore@0057852bfaa89a56745cba8c7296529d2fc39830', body)
        self.assertIn('uses: actions/cache/save@0057852bfaa89a56745cba8c7296529d2fc39830', body)
        self.assertIn("steps.go_cache_size.outputs.save == 'yes'", body)
        self.assertIn("GO_CACHE_MAX_BYTES: '1288490188'", body)
        self.assertIn('py -3 -B tools/ci/go_cache_key.py', body)
        self.assertIn('py -3 -B tools/ci/go_cache_save.py', body)
        for step in ('Identify Go build cache', 'Restore Go build cache',
                     'Check optional Go cache upload', 'Save Go build cache'):
            section = body.split('      - name: ' + step + '\n', 1)[1].split('\n      - ', 1)[0]
            self.assertIn('continue-on-error: true', section)


if __name__ == '__main__':
    unittest.main()
