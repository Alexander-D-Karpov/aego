"""Keep the website demo and both existing benchmark workloads distinct."""
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

import platformer_bench
import stage12_bench


ROOT = Path(__file__).resolve().parents[2]
SITE_BASELINE = ROOT / 'tools/websmoke/bench-site-demo-baseline.json'
SOURCE_SCENE_SHA256 = '6b3fd161a9638356cb413e2daf2c5efd68d218962fcc969b18d162a8c38926d7'
SITE_SCENE_SHA256 = '55c4f1f73436cd4cbb87346ef8d25261ce730d265ab6873188e5d41f22576870'


class BenchmarkWorkloadsTest(unittest.TestCase):
    def test_ci_keeps_three_distinct_gates(self):
        with tempfile.TemporaryDirectory(prefix='aego-bench-make-test-') as directory:
            env = dict(os.environ, HOME=directory, XDG_CONFIG_HOME=directory, GOTELEMETRY='off')
            for name in ('MAKEFLAGS', 'MFLAGS', 'MAKELEVEL', 'GNUMAKEFLAGS'):
                env.pop(name, None)
            result = subprocess.run(['make', '-f', 'Makefile', '-qp', 'GOOS=linux', 'GOARCH=amd64', 'ci'],
                                    cwd=ROOT, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                    text=True, stdin=subprocess.DEVNULL, timeout=30)
            self.assertIn(result.returncode, (0, 1), result.stderr)
            lines = result.stdout.splitlines()
            targets = [line for line in lines if line.startswith('ci: ') and ':=' not in line]
            self.assertEqual(len(targets), 1, targets)
            deps = set(targets[0].split()[1:])
            self.assertTrue({'web-smoke', 'web-bench-platformer', 'web-bench-stage12',
                             'web-platformer-fixture-test', 'web-stage12-fixture-test'} <= deps)
            assignments = [line for line in lines if line.startswith('ci: WEB_BENCH_BASELINE := ')]
            self.assertEqual(assignments, ['ci: WEB_BENCH_BASELINE := ' + str(SITE_BASELINE)])

    def test_original_baselines_and_replay_remain_pinned(self):
        self.assertEqual(platformer_bench.digest(platformer_bench.BASELINE), platformer_bench.BASELINE_SHA256)
        self.assertEqual(platformer_bench.digest(platformer_bench.REPLAY), platformer_bench.REPLAY_SHA256)
        self.assertEqual(stage12_bench.digest(stage12_bench.BASELINE), stage12_bench.BASELINE_SHA256)
        self.assertEqual(stage12_bench.digest(stage12_bench.FIXTURE), stage12_bench.FIXTURE_SHA256)

    def test_site_scene_and_its_reference_are_independently_pinned(self):
        scene = (ROOT / 'examples/platformer/scenes/main.aescene').read_bytes()
        self.assertEqual(hashlib.sha256(scene).hexdigest(), SOURCE_SCENE_SHA256)
        spec = importlib.util.spec_from_file_location('site_demo_prepare', ROOT / 'tools/site_demo_prepare.py')
        prepare = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(prepare)
        prepared = prepare.close_checkpoint_gap(scene.decode('utf-8')).encode('utf-8')
        self.assertEqual(hashlib.sha256(prepared).hexdigest(), SITE_SCENE_SHA256)
        baseline = json.loads(SITE_BASELINE.read_text())['swiftshader']
        self.assertEqual(baseline['meta']['viewport'], '1280x720')
        self.assertEqual((baseline['meta']['warmup'], baseline['meta']['frames']), (120, 600))
        result = baseline['results']['chromium']
        self.assertGreaterEqual(len(result['runs']), 5)
        for row in result['runs']:
            self.assertEqual(row['frames'], 600)
            self.assertEqual(row['errors'], [])
            self.assertIn('SwiftShader', row['renderer'])
            self.assertEqual(row['drawsPerFrame'], 14.835)
        old = json.loads(platformer_bench.BASELINE.read_text())['swiftshader']['results']['chromium']['summary']
        self.assertLessEqual(result['summary']['frameBytesPerFrame'], old['frameBytesPerFrame'])


if __name__ == '__main__':
    unittest.main()
