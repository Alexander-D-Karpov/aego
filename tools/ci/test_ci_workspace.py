"""Check prepared CI workspace attachment without touching real caches."""

import importlib.util
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
from unittest import mock


ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location('aego_ci_workspace', ROOT / 'tools/ci/prepare_ci_workspace.py')
WORKSPACE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(WORKSPACE)


class PreparedWorkspaceTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='aego-ci-workspace-')
        self.addCleanup(self.temp.cleanup)
        self.scratch = Path(self.temp.name)
        self.repo = self.scratch / 'repo'
        self.image = self.scratch / 'image'
        self.repo.mkdir()
        self.image.mkdir()
        for source, image_name in WORKSPACE.PREFLIGHT.INPUTS.items():
            for target in (self.repo / source, self.image / image_name):
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(ROOT / source, target)
        for path in (self.repo / 'tools/ci/env/lock.json', self.image / 'ci-env/lock.json'):
            path.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(ROOT / 'tools/ci/env/lock.json', path)
        (self.repo / 'go.mod').write_text('module fixture\ngo 1.27\ntoolchain go1.27.0\n')
        self.sdl = self.image / 'native/linux_amd64'
        (self.sdl / 'lib').mkdir(parents=True)
        (self.sdl / 'lib/libSDL3.a').write_bytes(b'synthetic library')
        (self.sdl / '.sdl-3.2.24').write_bytes(b'')
        self.modules = self.image / 'websmoke/node_modules'
        self.modules.mkdir()
        (self.modules / 'fixture').write_text('dependency fixture')

    def test_prepares_and_reuses_exact_private_dependencies(self):
        first = WORKSPACE.prepare(self.repo, self.image)
        self.assertTrue(first['sdl_copied'])
        self.assertTrue(first['node_link_created'])
        dest = self.repo / '.cache/native/linux_amd64'
        self.assertEqual(WORKSPACE.tree(dest), WORKSPACE.tree(self.sdl))
        self.assertEqual((self.repo / 'tools/websmoke/node_modules').resolve(), self.modules.resolve())
        second = WORKSPACE.prepare(self.repo, self.image)
        self.assertFalse(second['sdl_copied'])
        self.assertFalse(second['node_link_created'])

    def test_prepared_dependency_link_is_ignored_as_generated_output(self):
        subprocess.run(['git', 'init', '-q', str(self.repo)], check=True)
        shutil.copyfile(ROOT / '.gitignore', self.repo / '.gitignore')
        WORKSPACE.prepare(self.repo, self.image)
        link = self.repo / 'tools/websmoke/node_modules'
        self.assertTrue(link.is_symlink())
        self.assertEqual(link.resolve(), self.modules.resolve())
        status = subprocess.check_output(['git', 'status', '--porcelain=v1', '--untracked-files=all',
                                          '--', 'tools/websmoke/node_modules'], cwd=self.repo)
        self.assertEqual(status, b'', 'prepared dependency link must not make a clean CI checkout dirty')

    def test_conflicting_node_modules_are_not_replaced_or_touched(self):
        modules = self.repo / 'tools/websmoke/node_modules'
        modules.mkdir()
        (modules / 'user-data').write_text('keep')
        with self.assertRaisesRegex(ValueError, 'Node dependencies differ'):
            WORKSPACE.prepare(self.repo, self.image)
        self.assertEqual((modules / 'user-data').read_text(), 'keep')
        self.assertFalse((self.repo / '.cache').exists())

    def test_conflicting_sdl_tree_is_preserved(self):
        dest = self.repo / '.cache/native/linux_amd64'
        dest.mkdir(parents=True)
        (dest / 'user-data').write_text('keep')
        with self.assertRaisesRegex(ValueError, 'SDL tree differs'):
            WORKSPACE.prepare(self.repo, self.image)
        self.assertEqual((dest / 'user-data').read_text(), 'keep')
        self.assertFalse((self.repo / 'tools/websmoke/node_modules').exists())

    def test_symlinked_cache_and_changed_lock_are_rejected(self):
        external = self.scratch / 'external'
        external.mkdir()
        (self.repo / '.cache').symlink_to(external, target_is_directory=True)
        with self.assertRaisesRegex(ValueError, 'workspace directory'):
            WORKSPACE.prepare(self.repo, self.image)
        self.assertEqual(list(external.iterdir()), [])
        (self.repo / '.cache').unlink()
        (self.repo / 'tools/ci/env/lock.json').write_text('{}')
        with self.assertRaisesRegex(ValueError, 'locks differ'):
            WORKSPACE.prepare(self.repo, self.image)
        self.assertFalse((self.repo / '.cache').exists())

    def test_linked_or_special_sdl_inputs_are_rejected(self):
        (self.sdl / 'linked').symlink_to(self.sdl / '.sdl-3.2.24')
        with self.assertRaisesRegex(ValueError, 'symlink or special file'):
            WORKSPACE.prepare(self.repo, self.image)
        self.assertFalse((self.repo / '.cache').exists())

    def test_unreadable_sdl_subtree_is_not_accepted_as_a_complete_copy(self):
        def interrupted(root, *, followlinks=False, onerror=None):
            yield str(root), ['lib'], []
            if onerror is not None:
                onerror(PermissionError('unreadable prepared SDL subtree'))
        with mock.patch.object(WORKSPACE.os, 'walk', side_effect=interrupted):
            with self.assertRaisesRegex(PermissionError, 'unreadable prepared SDL'):
                WORKSPACE.tree(self.sdl)


if __name__ == '__main__':
    unittest.main()
