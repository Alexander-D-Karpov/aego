import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch


spec = importlib.util.spec_from_file_location('platformer_bench', Path(__file__).with_name('platformer_bench.py'))
bench = importlib.util.module_from_spec(spec)
spec.loader.exec_module(bench)


class PlatformerBenchTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix='aego-platformer-bench-test-')
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name) / 'repo'
        self.root.mkdir()
        self.dest = Path(self.tmp.name) / 'copy'
        self.dest.mkdir()
        self.env = dict(os.environ, GIT_CONFIG_GLOBAL='/dev/null', GIT_CONFIG_NOSYSTEM='1',
                        GIT_AUTHOR_NAME='Test', GIT_AUTHOR_EMAIL='test@example.invalid',
                        GIT_COMMITTER_NAME='Test', GIT_COMMITTER_EMAIL='test@example.invalid')
        self.git('init', '-q')
        self.project = self.root / 'examples/platformer'
        (self.project / 'scenes').mkdir(parents=True)
        (self.project / 'assets').mkdir()
        (self.project / 'go.mod').write_text('module example.invalid/game\n')
        (self.project / 'scenes/main.aescene').write_bytes(b'original scene\n')
        (self.project / 'assets/tracked.png').write_bytes(b'tracked pixels\n')
        (self.project / '.gitignore').write_text('.aego/\n')
        self.git('add', '.')
        self.git('commit', '-qm', 'fixture')

    def git(self, *args):
        return subprocess.check_output(['git', *args], cwd=self.root, env=self.env, stderr=subprocess.PIPE)

    def test_copies_only_tracked_inputs_without_touching_user_files(self):
        (self.project / 'assets/user.png').write_bytes(b'user pixels')
        (self.project / '.aego').mkdir()
        (self.project / '.aego/recovery').write_bytes(b'user recovery')
        before = self.git('status', '--porcelain', '-uall')
        copied = bench.copy_tracked_project(self.root, self.dest)
        self.assertEqual((self.dest / 'scenes/main.aescene').read_bytes(), b'original scene\n')
        self.assertEqual((self.dest / 'assets/tracked.png').read_bytes(), b'tracked pixels\n')
        self.assertNotIn('examples/platformer/assets/user.png', copied)
        self.assertFalse((self.dest / 'assets/user.png').exists())
        self.assertFalse((self.dest / '.aego').exists())
        self.assertEqual((self.project / '.aego/recovery').read_bytes(), b'user recovery')
        self.assertEqual(self.git('status', '--porcelain', '-uall'), before)

    def test_dirty_and_staged_inputs_are_rejected(self):
        scene = self.project / 'scenes/main.aescene'
        scene.write_bytes(b'changed')
        with self.assertRaisesRegex(ValueError, 'match HEAD'):
            bench.copy_tracked_project(self.root, self.dest)
        self.git('add', 'examples/platformer/scenes/main.aescene')
        scene.write_bytes(b'original scene\n')  # Worktree matches HEAD but index is different.
        with self.assertRaisesRegex(ValueError, 'match HEAD'):
            bench.copy_tracked_project(self.root, self.dest)
        self.assertEqual(scene.read_bytes(), b'original scene\n')
        self.assertEqual(list(self.dest.iterdir()), [])

    def test_symlinks_and_nonempty_destination_are_rejected(self):
        tracked = self.project / 'assets/tracked.png'
        tracked.unlink()
        tracked.symlink_to('../go.mod')
        self.git('add', 'examples/platformer/assets/tracked.png')
        self.git('commit', '-qm', 'tracked symlink')
        with self.assertRaisesRegex(ValueError, 'regular'):
            bench.copy_tracked_project(self.root, self.dest)
        self.assertTrue(tracked.is_symlink())
        (self.dest / 'user').write_bytes(b'keep')
        with self.assertRaisesRegex(ValueError, 'new empty'):
            bench.copy_tracked_project(self.root, self.dest)
        self.assertEqual((self.dest / 'user').read_bytes(), b'keep')

    def test_copy_budget_and_source_change_are_rejected(self):
        with patch.object(bench, 'MAX_FILE_BYTES', 3), self.assertRaisesRegex(ValueError, 'copy budget'):
            bench.copy_tracked_project(self.root, self.dest)
        self.assertEqual(list(self.dest.iterdir()), [])
        scene = self.project / 'scenes/main.aescene'
        digest = bench.digest
        changed = False

        def change_after_hash(path, limit=None):
            nonlocal changed
            result = digest(path, limit)
            if path == scene and not changed:
                changed = True
                scene.write_bytes(b'concurrent edit')
            return result

        with patch.object(bench, 'digest', side_effect=change_after_hash):
            with self.assertRaisesRegex(ValueError, 'changed while being copied'):
                bench.copy_tracked_project(self.root, self.dest)
        self.assertEqual(scene.read_bytes(), b'concurrent edit')

    def test_received_bytes_and_complete_samples_are_required(self):
        demo = Path(self.tmp.name) / 'demo'
        (demo / 'data').mkdir(parents=True)
        (demo / 'game.wasm').write_bytes(b'wasm')
        (demo / 'data/manifest.aetext').write_bytes(b'manifest')
        receipt = {'game.wasm':bench.digest(demo / 'game.wasm'),
                   'manifest.aetext':bench.digest(demo / 'data/manifest.aetext')}
        row = {'frames':600, 'errors':[], 'receipts':receipt}
        report = {'results':{'chromium':{'runs':[row]}}}
        self.assertEqual(bench.verify_receipts(report, demo, 1), receipt)
        for bad in ({'frames':599, 'errors':[], 'receipts':receipt},
                    {'frames':600, 'errors':['bad'], 'receipts':receipt},
                    {'frames':600, 'errors':[], 'receipts':dict(receipt, **{'game.wasm':'wrong'})}):
            with self.subTest(row=json.dumps(bad)), self.assertRaises(ValueError):
                bench.verify_receipts({'results':{'chromium':{'runs':[bad]}}}, demo, 1)
        with self.assertRaises(ValueError):
            bench.verify_receipts(report, demo, 2)

    def test_pinned_original_baseline_and_replay_are_present(self):
        self.assertEqual(bench.digest(bench.BASELINE), bench.BASELINE_SHA256)
        self.assertEqual(bench.digest(bench.REPLAY), bench.REPLAY_SHA256)

    def test_inventory_hash_is_order_independent_and_content_sensitive(self):
        first = {'examples/platformer/a': '1' * 64, 'examples/platformer/b': '2' * 64}
        second = dict(reversed(list(first.items())))
        self.assertEqual(bench.inventory_digest(first), bench.inventory_digest(second))
        second['examples/platformer/a'] = '3' * 64
        self.assertNotEqual(bench.inventory_digest(first), bench.inventory_digest(second))


if __name__ == '__main__':
    unittest.main()
