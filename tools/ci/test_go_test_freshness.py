"""Ensure cached Go builds cannot replace required CI test executions."""

import os
from pathlib import Path
import re
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
TARGETS = {
    'test-race-packages': 1,
    'test-editor': 1,
    'test-runtime': 1,
    'test-wasm': 7,
    'test-nonative': 1,
    'test-gl': 6,
    'golden': 1,
    'test-animation': 1,
    'test-camera': 2,
    'test-gameplay': 1,
    'demo-physics': 1,
}


def go_test_lines(text):
    return [line for line in text.splitlines() if re.search(r'\bgo\s+test\b', line)]


def require_fresh(lines):
    if not lines or any(not re.search(r'(?<!\S)-count(?:=|\s+)1(?:\s|$)', line) for line in lines):
        raise ValueError('a required Go test may use cached test results')


class GoTestFreshnessTest(unittest.TestCase):
    def test_make_ci_go_tests_are_fresh_without_running_them(self):
        with tempfile.TemporaryDirectory(prefix='aego-go-freshness-') as scratch:
            env = dict(os.environ, HOME=scratch, XDG_CONFIG_HOME=scratch, GOTELEMETRY='off')
            for name in ('MAKEFLAGS', 'MFLAGS', 'MAKELEVEL', 'GNUMAKEFLAGS'):
                env.pop(name, None)
            for target, minimum in TARGETS.items():
                with self.subTest(target=target):
                    result = subprocess.run(['make', '-n', '-f', str(ROOT / 'Makefile'), '-o', 'bootstrap',
                                             'GOOS=linux', 'GOARCH=amd64', 'XVFB=', target],
                                            cwd=ROOT, env=env, text=True, stdout=subprocess.PIPE,
                                            stderr=subprocess.STDOUT, stdin=subprocess.DEVNULL, timeout=30)
                    self.assertEqual(result.returncode, 0, result.stdout)
                    lines = go_test_lines(result.stdout)
                    self.assertGreaterEqual(len(lines), minimum, result.stdout)
                    require_fresh(lines)

    def test_gitea_build_job_go_tests_are_fresh(self):
        workflow = (ROOT / '.github/workflows/ci.yml').read_text()
        job = workflow.split('\n  build:\n', 1)[1].split('\n  release:\n', 1)[0]
        lines = go_test_lines(job)
        self.assertEqual(len(lines), 4, lines)
        require_fresh(lines)

    def test_optional_gitea_platform_go_tests_are_fresh(self):
        workflow = (ROOT / '.github/workflows/ci-platforms.yml').read_text()
        lines = go_test_lines(workflow)
        self.assertEqual(len(lines), 6, lines)
        require_fresh(lines)

    def test_missing_or_nonfresh_count_is_rejected(self):
        for text in ('go test ./...', 'go test -count=2 ./...', ''):
            with self.subTest(text=text), self.assertRaises(ValueError):
                require_fresh(go_test_lines(text))
        require_fresh(go_test_lines('go test -race -count=1 ./...'))


if __name__ == '__main__':
    unittest.main()
