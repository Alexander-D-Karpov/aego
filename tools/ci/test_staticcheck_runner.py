"""Verify prepared Staticcheck selection and local argument compatibility."""

import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest import mock


ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / 'tools/ci/staticcheck_runner.py'
SPEC = importlib.util.spec_from_file_location('aego_staticcheck_runner', SCRIPT)
RUNNER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(RUNNER)


class StaticcheckRunnerTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='aego-staticcheck-runner-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.repo = self.root / 'repo'
        self.image = self.root / 'image'
        self.repo.mkdir()
        self.image.mkdir()
        for source, image_name in RUNNER.PREFLIGHT.INPUTS.items():
            for target in (self.repo / source, self.image / image_name):
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(ROOT / source, target)
        for target in (self.repo / 'tools/ci/env/lock.json', self.image / 'ci-env/lock.json'):
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(ROOT / 'tools/ci/env/lock.json', target)
        self.binary = self.image / 'bin/staticcheck'
        self.binary.parent.mkdir()
        self.binary.write_text('#!/bin/sh\nexit 0\n')
        self.binary.chmod(0o755)

    def test_local_mode_preserves_exact_go_run_arguments(self):
        args = ['-tags', 'aego_editor', './editor/...', './app/...', './tests/...']
        self.assertEqual(RUNNER.select_command(self.repo, self.image, 'v0.8.2', args, '0'),
                         ['go', 'run', 'honnef.co/go/tools/cmd/staticcheck@v0.8.2', *args])
        fakebin = self.root / 'bin'
        fakebin.mkdir()
        go = fakebin / 'go'
        go.write_text('#!/bin/sh\nprintf "%s\\n" "$@"\n')
        go.chmod(0o755)
        result = subprocess.run([sys.executable, '-B', str(SCRIPT), 'v0.8.2', '--', *args],
                                cwd=self.repo, env=dict(os.environ, PATH=str(fakebin) + os.pathsep + os.defpath,
                                                       AEGO_CI_PREPARED='0'),
                                text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=10)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.splitlines(), ['run', 'honnef.co/go/tools/cmd/staticcheck@v0.8.2', *args])

    def test_prepared_mode_uses_binary_only_after_checks(self):
        args = ['-tags', 'aego_editor', './editor/...']
        with mock.patch.object(RUNNER.PREFLIGHT, 'check_tools') as check:
            command = RUNNER.select_command(self.repo, self.image, 'v0.8.2', args, '1')
        self.assertEqual(command, [str(self.binary), *args])
        check.assert_called_once()

    def test_make_lint_preserves_both_original_flag_sets(self):
        fakebin = self.root / 'make-bin'
        fakebin.mkdir()
        trace = self.root / 'make-trace'
        go = fakebin / 'go'
        go.write_text('#!/bin/sh\nprintf "%s\\n" "$*" >> "$TRACE"\n')
        go.chmod(0o755)
        env = dict(os.environ, PATH=str(fakebin) + os.pathsep + os.defpath,
                   AEGO_CI_PREPARED='0', TRACE=str(trace))
        for name in ('MAKEFLAGS', 'MFLAGS', 'MAKELEVEL', 'GNUMAKEFLAGS'):
            env.pop(name, None)
        result = subprocess.run(['make', '-f', str(ROOT / 'Makefile'), '-o', 'bootstrap',
                                 'GOOS=linux', 'GOARCH=amd64', 'LINT_PKGS=aego/one aego/two', 'lint'],
                                cwd=ROOT, env=env, text=True, stdout=subprocess.PIPE,
                                stderr=subprocess.STDOUT, timeout=15)
        self.assertEqual(result.returncode, 0, result.stdout)
        self.assertEqual(trace.read_text().splitlines(), [
            'run honnef.co/go/tools/cmd/staticcheck@v0.8.2 aego/one aego/two',
            'run honnef.co/go/tools/cmd/staticcheck@v0.8.2 -tags aego_editor ./editor/... ./app/... ./tests/...',
        ])

    def test_invalid_mode_version_lock_or_binary_fails_before_execution(self):
        for mode, version in (('invalid', 'v0.8.2'), ('1', 'v9.0.0')):
            with self.subTest(mode=mode, version=version), self.assertRaises(ValueError):
                RUNNER.select_command(self.repo, self.image, version, ['./...'], mode)
        self.binary.unlink()
        with self.assertRaisesRegex(ValueError, 'binary is missing'):
            RUNNER.select_command(self.repo, self.image, 'v0.8.2', ['./...'], '1')
        lock = self.repo / 'tools/ci/env/lock.json'
        lock.write_text(json.dumps({'schema': 0}))
        with self.assertRaisesRegex(ValueError, 'locks differ'):
            RUNNER.select_command(self.repo, self.image, 'v0.8.2', ['./...'], '1')


if __name__ == '__main__':
    unittest.main()
