"""Test CI shortcut dispatch without running expensive product checks."""

import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]


class CIShortcutTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='aego-ci-shortcut-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        self.trace = self.root / 'trace'
        self.outer_make = shutil.which('make')
        if not self.outer_make:
            self.fail('make is required')
        stub = self.bin / 'make'
        stub.write_text('#!/bin/sh\nprintf "%s\\n" "$*" >> "$TRACE"\nexit "${STUB_EXIT:-0}"\n')
        stub.chmod(0o755)
        self.env = dict(os.environ, PATH=str(self.bin) + os.pathsep + os.defpath, TRACE=str(self.trace),
                        HOME=str(self.root), XDG_CONFIG_HOME=str(self.root), GOTELEMETRY='off')
        for name in ('MAKEFLAGS', 'MFLAGS', 'MAKELEVEL', 'GNUMAKEFLAGS', 'STUB_EXIT'):
            self.env.pop(name, None)

    def run_make(self, *args):
        return subprocess.run([self.outer_make, '-f', str(ROOT / 'Makefile'),
                               'GOOS=linux', 'GOARCH=amd64', *args], cwd=ROOT, env=self.env,
                              text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=20)

    def test_suite_dispatches_only_allowlisted_targets(self):
        cases = {'infra': 'ci-infrastructure-test', 'integration-race': 'test-race-integration',
                 'studio-core': 'studio-web-test', 'component-try': 'component-try',
                 'gl': 'test-gl', 'web-gpu': 'web-gpu-smoke', 'remote-examples': 'remote-examples-check'}
        for suite, target in cases.items():
            with self.subTest(suite=suite):
                if self.trace.exists():
                    self.trace.unlink()
                result = self.run_make('ci-suite', 'SUITE=' + suite)
                self.assertEqual(result.returncode, 0, result.stdout)
                self.assertEqual(self.trace.read_text().splitlines(), [target])
                self.assertIn('not full CI', result.stdout)

    def test_unknown_suite_fails_without_running_make(self):
        result = self.run_make('ci-suite', 'SUITE=unknown;touch ' + str(self.root / 'injected'))
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertFalse(self.trace.exists())
        self.assertFalse((self.root / 'injected').exists())

    def test_suite_failure_propagates(self):
        self.env['STUB_EXIT'] = '37'
        result = self.run_make('ci-suite', 'SUITE=gl')
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertEqual(self.trace.read_text().splitlines(), ['test-gl'])

    def test_fast_and_full_have_explicit_distinct_dependencies(self):
        result = self.run_make('-qp', 'ci-fast', 'ci-full')
        self.assertIn(result.returncode, (0, 1), result.stdout)
        fast = re.findall(r'^ci-fast:\s*([^\n]*)$', result.stdout, re.MULTILINE)
        full = re.findall(r'^ci-full:\s*([^\n]*)$', result.stdout, re.MULTILINE)
        self.assertEqual(len(fast), 1)
        self.assertEqual(len(full), 1)
        self.assertIn('ci-infrastructure-test', fast[0].split())
        self.assertNotIn('ci', fast[0].split())
        self.assertEqual(full[0].split(), ['ci'])


if __name__ == '__main__':
    unittest.main()
