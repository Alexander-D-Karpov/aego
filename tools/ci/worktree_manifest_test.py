import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

import worktree_manifest as manifest


class WorktreeManifestTest(unittest.TestCase):
    def test_tracks_bytes_modes_missing_and_links_without_following_them(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'tracked').write_bytes(b'first')
            (root / 'untracked').write_bytes(b'untracked')
            os.symlink('/does/not/exist', root / 'link')
            names = b'untracked\0missing\0tracked\0link\0tracked\0'

            def git(_root, *args):
                return b'1234567890abcdef\n' if args[0] == 'rev-parse' else names

            with mock.patch.object(manifest, 'git', side_effect=git):
                first = manifest.inventory(root)
                self.assertEqual(first, manifest.inventory(root))
                self.assertEqual(first['fileCount'], 4)
                self.assertEqual([f['path'] for f in first['files']],
                                 ['link', 'missing', 'tracked', 'untracked'])
                self.assertEqual(first['files'][0]['kind'], 'symlink')
                self.assertEqual(first['files'][1]['kind'], 'missing')
                (root / 'tracked').write_bytes(b'second')
                self.assertNotEqual(first['worktreeSHA256'], manifest.inventory(root)['worktreeSHA256'])
                (root / 'tracked').write_bytes(b'first')
                (root / 'tracked').chmod(0o755)
                self.assertNotEqual(first['worktreeSHA256'], manifest.inventory(root)['worktreeSHA256'])

    def test_rejects_special_entries(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'directory').mkdir()
            with mock.patch.object(manifest, 'git', side_effect=[b'head\n', b'directory\0']):
                with self.assertRaisesRegex(RuntimeError, 'unsupported worktree entry'):
                    manifest.inventory(root)

    def test_snapshot_runner_does_not_write_local_python_bytecode(self):
        tools = Path(__file__).resolve().parents[1]
        helpers = tools / 'artstudio' / 'release' / 'tests'
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            baseline, probe = root / 'baseline', root / 'probe'
            baseline.mkdir()
            probe.mkdir()
            for name in ('authoring_ui.py', 'effect_templates_ui.py'):
                shutil.copyfile(helpers / name, baseline / name)
                shutil.copyfile(helpers / name, probe / name)
            env = os.environ.copy()
            env.pop('PYTHONDONTWRITEBYTECODE', None)
            env.pop('PYTHONPYCACHEPREFIX', None)
            subprocess.run([sys.executable, '-c', 'import authoring_ui; import effect_templates_ui'],
                           cwd=baseline, env=env, check=True)
            self.assertEqual(len(list((baseline / '__pycache__').glob('*.pyc'))), 2)

            # Stop the real runner at its first Python invocation, but let the
            # child interpreter import the same helpers from a fresh directory.
            wrapper = root / 'python-probe'
            wrapper.write_text('''#!/usr/bin/env python3
import json, os, sys
from pathlib import Path
assert sys.argv[1:] == ['-c', 'import playwright.sync_api']
sys.path.insert(0, os.environ['HELPER_DIR'])
import authoring_ui
import effect_templates_ui
root = Path(os.environ['HELPER_DIR'])
Path(os.environ['PROBE_RESULT']).write_text(json.dumps({
    'setting': os.environ.get('PYTHONDONTWRITEBYTECODE'),
    'bytecode': [str(path.relative_to(root)) for path in root.rglob('*.pyc')],
}))
raise SystemExit(73)
''', encoding='utf-8')
            wrapper.chmod(0o755)
            result = root / 'result.json'
            env.update(ARTSTUDIO_PYTHON=str(wrapper), ARTSTUDIO_SNAPSHOT_DIR=str(root / 'evidence'),
                       HELPER_DIR=str(probe), PROBE_RESULT=str(result))
            runner = tools / 'ci' / 'artstudio_snapshot_e2e.sh'
            completed = subprocess.run(['bash', str(runner)], env=env, capture_output=True, text=True)
            self.assertEqual(completed.returncode, 73, completed.stderr)
            self.assertEqual(json.loads(result.read_text()), {'setting': '1', 'bytecode': []})


if __name__ == '__main__':
    unittest.main()
