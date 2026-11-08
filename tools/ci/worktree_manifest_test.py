import os
from pathlib import Path
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


if __name__ == '__main__':
    unittest.main()
