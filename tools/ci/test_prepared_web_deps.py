"""Check prepared web dependency selection with isolated Node fixture packages."""

import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location('aego_prepared_web_deps', ROOT / 'tools/ci/prepared_web_deps.py')
WEB = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(WEB)


class PreparedWebDepsTest(unittest.TestCase):
    def setUp(self):
        if not shutil.which('node'):
            self.skipTest('Node is unavailable')
        self.temp = tempfile.TemporaryDirectory(prefix='aego-prepared-web-')
        self.addCleanup(self.temp.cleanup)
        self.scratch = Path(self.temp.name)
        self.repo = self.scratch / 'repo'
        self.image = self.scratch / 'image'
        self.repo.mkdir()
        self.image.mkdir()
        for source, image_name in WEB.PREFLIGHT.INPUTS.items():
            for target in (self.repo / source, self.image / image_name):
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(ROOT / source, target)
        lock = json.loads((ROOT / 'tools/ci/env/lock.json').read_text())
        lock['tools']['node'] = subprocess.check_output(['node', '--version'], text=True, timeout=10).strip()
        for path in (self.repo / 'tools/ci/env/lock.json', self.image / 'ci-env/lock.json'):
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(json.dumps(lock))
        self.modules = self.image / 'websmoke/node_modules'
        packages = json.loads((self.repo / 'tools/websmoke/package-lock.json').read_text())['packages']
        for name in WEB.PACKAGES:
            directory = self.modules / name
            directory.mkdir(parents=True)
            version = packages['node_modules/' + name]['version']
            (directory / 'package.json').write_text(json.dumps({'name': name, 'version': version, 'main': 'index.js'}))
            (directory / 'index.js').write_text('module.exports = {};\n')
        (self.repo / 'tools/websmoke/node_modules').symlink_to(self.modules, target_is_directory=True)

    def test_imports_exact_packages_through_workspace_symlink(self):
        result = WEB.verify(self.repo, self.image)
        self.assertEqual(result['status'], 'PASS')
        self.assertEqual(result['packages'], {'playwright-core': '1.64.1', 'pixelmatch': '6.0.0', 'pngjs': '7.0.0'})
        self.assertFalse((self.repo / '.cache').exists())

    def test_changed_or_missing_package_fails_without_installation(self):
        manifest = self.modules / 'pixelmatch/package.json'
        manifest.write_text(json.dumps({'name': 'pixelmatch', 'version': '0.0.1', 'main': 'index.js'}))
        with self.assertRaisesRegex(ValueError, 'package versions differ'):
            WEB.verify(self.repo, self.image)
        manifest.write_text(json.dumps({'name': 'pixelmatch', 'version': '6.0.0', 'main': 'index.js'}))
        (self.modules / 'pixelmatch/index.js').unlink()
        with self.assertRaisesRegex(ValueError, 'dependency import failed'):
            WEB.verify(self.repo, self.image)
        self.assertFalse((self.repo / '.cache').exists())

    def test_unrelated_node_modules_and_changed_inputs_are_rejected(self):
        link = self.repo / 'tools/websmoke/node_modules'
        link.unlink()
        link.mkdir()
        (link / 'user-data').write_text('keep')
        with self.assertRaisesRegex(ValueError, 'workspace does not use'):
            WEB.verify(self.repo, self.image)
        self.assertEqual((link / 'user-data').read_text(), 'keep')
        source = self.repo / 'tools/websmoke/package-lock.json'
        source.write_bytes(source.read_bytes() + b' ')
        with self.assertRaises(ValueError):
            WEB.verify(self.repo, self.image)
        self.assertEqual((link / 'user-data').read_text(), 'keep')


if __name__ == '__main__':
    unittest.main()
