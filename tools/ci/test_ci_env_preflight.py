"""Offline contract tests for the candidate prepared CI environment."""

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
SPEC = importlib.util.spec_from_file_location('aego_ci_env_preflight', ROOT / 'tools/ci/env/preflight.py')
PREFLIGHT = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(PREFLIGHT)
LOCK = json.loads((ROOT / 'tools/ci/env/lock.json').read_text(encoding='utf-8'))


class EnvironmentInputTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='aego-ci-env-test-')
        self.addCleanup(self.temp.cleanup)
        self.scratch = Path(self.temp.name)
        self.root = self.scratch / 'repo'
        self.root.mkdir()
        for name in PREFLIGHT.INPUTS:
            target = self.root / name
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(ROOT / name, target)
        shutil.copyfile(ROOT / 'tools/ci/env/lock.json', self.root / 'tools/ci/env/lock.json')
        (self.root / 'go.mod').write_text('module fixture\n\ngo 1.27\n\ntoolchain go1.27.0\n')

    def test_lock_matches_all_current_and_fixture_inputs(self):
        PREFLIGHT.check_input_set(ROOT, LOCK)
        PREFLIGHT.check_input_set(self.root, LOCK)
        PREFLIGHT.check_go_directives(self.root, LOCK['tools']['go'])
        dockerfile = (self.root / 'tools/ci/env/Dockerfile').read_text()
        for image in ('base_image', 'go_image', 'node_image'):
            self.assertIn('FROM ' + LOCK[image], dockerfile)
        package = json.loads((self.root / 'tools/websmoke/package-lock.json').read_text())
        self.assertEqual(package['packages']['node_modules/playwright-core']['version'], LOCK['tools']['playwright'])

    def test_each_changed_or_missing_input_is_rejected(self):
        for name in PREFLIGHT.INPUTS:
            with self.subTest(name=name):
                path = self.root / name
                original = path.read_bytes()
                path.write_bytes(original + b'changed')
                with self.assertRaisesRegex(ValueError, name):
                    PREFLIGHT.check_input_set(self.root, LOCK)
                path.unlink()
                with self.assertRaisesRegex(ValueError, 'regular file'):
                    PREFLIGHT.check_input_set(self.root, LOCK)
                path.write_bytes(original)

    def test_unknown_sdl_patches_and_symlinked_inputs_are_rejected(self):
        patches = self.root / 'third_party/sdl/patches'
        patches.mkdir()
        (patches / 'new.patch').write_text('patch')
        with self.assertRaisesRegex(ValueError, 'SDL patch inputs changed'):
            PREFLIGHT.check_input_set(self.root, LOCK)
        (patches / 'new.patch').unlink()
        path = self.root / 'tools/bootstrap-native.sh'
        source = self.root / 'bootstrap-copy'
        source.write_bytes(path.read_bytes())
        path.unlink()
        path.symlink_to(source)
        with self.assertRaisesRegex(ValueError, 'regular file'):
            PREFLIGHT.check_input_set(self.root, LOCK)

    def test_missing_or_extra_lock_inputs_are_rejected(self):
        for mode in ('missing', 'extra', 'bad-hash'):
            lock = json.loads(json.dumps(LOCK))
            if mode == 'missing':
                lock['inputs'].pop('tools/bootstrap-native.sh')
            elif mode == 'extra':
                lock['inputs']['extra'] = 'a' * 64
            else:
                lock['inputs']['tools/bootstrap-native.sh'] = 'abcd'
            with self.subTest(mode=mode), self.assertRaises(ValueError):
                PREFLIGHT.check_input_set(self.root, lock)

    def test_go_requirement_and_workspace_changes_fail_closed(self):
        for content in ('module fixture\n', 'go 1.28\n', 'go 1.27\ntoolchain go1.27.3\n',
                        'go 1.27\ngo 1.27\n', 'go 1.27\ntoolchain future\n'):
            with self.subTest(content=content):
                (self.root / 'go.mod').write_text(content)
                with self.assertRaises(ValueError):
                    PREFLIGHT.check_go_directives(self.root, 'go1.27.2')
        (self.root / 'go.mod').write_text('module fixture\ngo 1.27\ntoolchain go1.27.0\n')
        (self.root / 'go.work').write_text('go 1.27\n')
        with self.assertRaisesRegex(ValueError, 'go.work'):
            PREFLIGHT.check_go_directives(self.root, 'go1.27.2')

    def test_active_go_workspace_is_rejected_even_outside_checkout(self):
        for workspace in ('', 'off'):
            with self.subTest(workspace=workspace), mock.patch.object(PREFLIGHT, 'command', return_value=workspace) as call:
                PREFLIGHT.check_go_workspace(self.root)
                call.assert_called_once_with(['go', 'env', 'GOWORK'], cwd=self.root)
        with mock.patch.object(PREFLIGHT, 'command', return_value='/parent/go.work'):
            with self.assertRaisesRegex(ValueError, 'ambient GOWORK'):
                PREFLIGHT.check_go_workspace(self.root)

    def test_platform_and_go_cross_compilation_overrides_fail_closed(self):
        expected = {'GOOS': 'linux', 'GOARCH': 'amd64', 'GOHOSTOS': 'linux', 'GOHOSTARCH': 'amd64', 'CGO_ENABLED': '1'}
        node = json.dumps({'platform': 'linux', 'arch': 'x64'})
        with mock.patch.object(PREFLIGHT.platform, 'system', return_value='Linux'), \
             mock.patch.object(PREFLIGHT.platform, 'machine', return_value='x86_64'):
            for field in expected:
                bad = dict(expected, **{field: 'wrong'})
                with self.subTest(field=field), mock.patch.object(PREFLIGHT, 'command', side_effect=['amd64', json.dumps(bad), node]):
                    with self.assertRaisesRegex(ValueError, field):
                        PREFLIGHT.check_platform()
            with mock.patch.object(PREFLIGHT, 'command', side_effect=['amd64', json.dumps(expected), node]):
                self.assertEqual(PREFLIGHT.check_platform(), expected)
            with mock.patch.object(PREFLIGHT, 'command', side_effect=['arm64']):
                with self.assertRaisesRegex(ValueError, 'package architecture'):
                    PREFLIGHT.check_platform()
            with mock.patch.object(PREFLIGHT, 'command', side_effect=['amd64', json.dumps(expected),
                                                                      json.dumps({'platform': 'linux', 'arch': 'arm64'})]):
                with self.assertRaisesRegex(ValueError, 'Node platform'):
                    PREFLIGHT.check_platform()
        with mock.patch.object(PREFLIGHT.platform, 'system', return_value='Linux'), \
             mock.patch.object(PREFLIGHT.platform, 'machine', return_value='aarch64'):
            with self.assertRaisesRegex(ValueError, 'host architecture'):
                PREFLIGHT.check_platform()

    def test_apt_inventory_requires_all_packages_and_exact_glslang(self):
        records = {name: '1.0' for name in PREFLIGHT.APT_PACKAGES}
        records['glslang-tools'] = LOCK['tools']['glslang_apt']

        def output(values):
            return '\n'.join(f'{name}:amd64|installed|{version}' for name, version in values.items())

        with mock.patch.object(PREFLIGHT, 'command', return_value=output(records)):
            self.assertEqual(PREFLIGHT.check_apt(LOCK), records)
        missing = dict(records)
        missing.pop('xvfb')
        wrong = dict(records, **{'glslang-tools': '0.0'})
        for value in (output(missing), output(wrong), output(records) + '\nxvfb|not-installed|1.0'):
            with self.subTest(output=value[-100:]), mock.patch.object(PREFLIGHT, 'command', return_value=value):
                with self.assertRaises(ValueError):
                    PREFLIGHT.check_apt(LOCK)

    def test_cache_compression_tools_are_required(self):
        self.assertIn('zstd', PREFLIGHT.APT_PACKAGES)
        dockerfile = (self.root / 'tools/ci/env/Dockerfile').read_text()
        self.assertRegex(dockerfile, r'apt-get install[^\n]*\bzstd\b')
        with mock.patch.object(PREFLIGHT.shutil, 'which', side_effect=lambda name: None if name == 'zstdmt' else '/fixture/' + name):
            with self.assertRaisesRegex(ValueError, 'zstdmt'):
                PREFLIGHT.check_cache_tools()
        with mock.patch.object(PREFLIGHT, 'check_cache_tools', side_effect=ValueError('compression tools missing')):
            with self.assertRaisesRegex(ValueError, 'compression tools missing'):
                PREFLIGHT.check_tools(LOCK)

    def test_browser_smoke_is_bounded_and_preserves_both_headed_browsers(self):
        dockerfile = (self.root / 'tools/ci/env/Dockerfile').read_text()
        self.assertIn("timeout --kill-after=10s 180s xvfb-run", dockerfile)
        fake = self.scratch / 'fake-playwright/playwright'
        fake.mkdir(parents=True)
        (fake / '__init__.py').write_text('')
        (fake / 'sync_api.py').write_text('''import os
class Page:
    def set_default_timeout(self, value): assert value == 30000
    def set_content(self, value, timeout): assert timeout == 30000 and '<p>ready</p>' in value
    def title(self): return 'bad' if os.environ.get('BAD_TITLE') else 'Aego CI environment'
    def locator(self, name): assert name == 'p'; return self
    def inner_text(self): return 'ready'
class Browser:
    def new_page(self): return Page()
    def close(self): pass
class Kind:
    def __init__(self, name): self.name = name
    def launch(self, **options):
        assert options['headless'] is False and options['timeout'] == 30000
        if self.name == 'chromium': assert options['args'] == ['--no-sandbox']
        return Browser()
class Driver:
    chromium = Kind('chromium')
    firefox = Kind('firefox')
    def __enter__(self): return self
    def __exit__(self, *args): pass
def sync_playwright(): return Driver()
''')
        env = dict(os.environ, PYTHONPATH=str(fake.parent))
        env.pop('BAD_TITLE', None)
        script = self.root / 'tools/ci/env/browser_smoke.py'
        result = subprocess.run([sys.executable, '-B', str(script)], env=env, text=True,
                                stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=10)
        self.assertEqual(result.returncode, 0, result.stdout)
        self.assertIn('chromium: browser smoke passed', result.stdout)
        self.assertIn('firefox: browser smoke passed', result.stdout)
        env['BAD_TITLE'] = '1'
        result = subprocess.run([sys.executable, '-B', str(script)], env=env, text=True,
                                stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=10)
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertNotIn('firefox: browser smoke passed', result.stdout)

    def test_browser_resolution_mismatch_is_rejected(self):
        node = {'version': LOCK['tools']['playwright'], 'chromium': '/fixture/chrome', 'firefox': '/fixture/firefox'}
        python = {'chromium': '/wrong/chrome', 'firefox': '/fixture/firefox'}
        with mock.patch.object(PREFLIGHT, 'command', side_effect=[json.dumps(node), json.dumps(python)]):
            with self.assertRaisesRegex(ValueError, 'Python/Node browser'):
                PREFLIGHT.check_browsers(self.root, LOCK)

    def test_build_context_copies_only_verified_inputs_to_a_new_external_path(self):
        (self.root / 'user-data.txt').write_text('do not copy')
        output = self.scratch / 'context'
        builder = ROOT / 'tools/ci/env/build_context.py'
        args = [sys.executable, '-B', str(builder), '--root', str(self.root), '--out', str(output)]
        result = subprocess.run(args, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=15)
        self.assertEqual(result.returncode, 0, result.stderr)
        report = json.loads(result.stdout)
        self.assertEqual(report['status'], 'PASS')
        files = {p.relative_to(output).as_posix() for p in output.rglob('*') if p.is_file()}
        self.assertEqual(files, set(PREFLIGHT.INPUTS) | {'tools/ci/env/lock.json'})
        self.assertFalse((output / 'user-data.txt').exists())
        repeat = subprocess.run(args, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=15)
        self.assertNotEqual(repeat.returncode, 0)
        self.assertEqual(files, {p.relative_to(output).as_posix() for p in output.rglob('*') if p.is_file()})
        inside = subprocess.run(args[:-1] + [str(self.root / 'must-not-create')], text=True,
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=15)
        self.assertNotEqual(inside.returncode, 0)
        self.assertFalse((self.root / 'must-not-create').exists())


if __name__ == '__main__':
    unittest.main()
