"""Verify that compiler, system and SDL changes invalidate Go build caches."""

import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest import mock


ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location('aego_go_cache_key', ROOT / 'tools/ci/go_cache_key.py')
KEY = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(KEY)
SAVE_SPEC = importlib.util.spec_from_file_location('aego_go_cache_save', ROOT / 'tools/ci/go_cache_save.py')
SAVE = importlib.util.module_from_spec(SAVE_SPEC)
SAVE_SPEC.loader.exec_module(SAVE)


class CacheKeyTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='aego-go-cache-key-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.native = self.root / '.cache/native/linux_amd64'
        (self.native / 'lib').mkdir(parents=True)
        (self.native / 'lib/libSDL3.a').write_bytes(b'fixture-sdl')
        self.gocache = self.root / 'cache'
        self.modcache = self.root / 'modules'
        self.go = {name: '' for name in KEY.GO_FIELDS}
        self.go.update(GOVERSION='go1.27.2', GOROOT='/toolchain', GOOS='linux', GOARCH='amd64',
                       CGO_ENABLED='1', CC='fixture-cc', CXX='fixture-cxx', GOCACHE=str(self.gocache),
                       GOMODCACHE=str(self.modcache))
        self.packages = 'gcc|installed|1\nlibc|installed|2'

    def collect(self):
        def command(args):
            if args[0] == 'go':
                return json.dumps(self.go)
            if args[0] == 'dpkg-query':
                return self.packages
            raise AssertionError(args)
        with mock.patch.object(KEY, 'command', side_effect=command), \
             mock.patch.object(KEY, 'compiler_identity', side_effect=lambda name: {'binary': name}), \
             mock.patch.object(KEY.platform, 'system', return_value='Linux'), \
             mock.patch.object(KEY.platform, 'platform', return_value='fixture-host'):
            return KEY.identity(self.root)

    def test_identical_inputs_are_stable_and_changes_miss(self):
        original, path, modules = self.collect()
        self.assertEqual(path, str(self.gocache))
        self.assertEqual(modules, str(self.modcache))
        self.assertEqual(self.collect()[0], original)
        self.go['CGO_CFLAGS'] = '-DCHANGED=1'
        self.assertNotEqual(self.collect()[0], original)
        self.go['CGO_CFLAGS'] = ''
        self.packages += '\nlibgl|installed|3'
        self.assertNotEqual(self.collect()[0], original)
        self.packages = 'gcc|installed|1\nlibc|installed|2'
        (self.native / 'lib/libSDL3.a').write_bytes(b'different-sdl')
        self.assertNotEqual(self.collect()[0], original)

    def test_missing_or_linked_native_input_is_rejected(self):
        (self.native / 'lib/libSDL3.a').unlink()
        (self.native / 'lib/libSDL3.a').symlink_to('/tmp')
        with self.assertRaisesRegex(ValueError, 'link or special'):
            self.collect()

    def test_unreadable_sdl_subtree_never_yields_a_cache_key(self):
        def interrupted(root, *, followlinks=False, onerror=None):
            yield str(root), ['lib'], []
            if onerror is not None:
                onerror(PermissionError('unreadable SDL subtree'))
        with mock.patch.object(KEY.os, 'walk', side_effect=interrupted):
            with self.assertRaisesRegex(PermissionError, 'unreadable SDL'):
                KEY.native_tree(self.native)

    def test_bad_cache_paths_or_platform_never_return_a_key(self):
        for value in ('', '/', 'relative', '/tmp/invalid\npath'):
            with self.subTest(value=value):
                self.go['GOCACHE'] = value
                with self.assertRaises(ValueError):
                    self.collect()
        self.go['GOCACHE'] = str(self.gocache)
        self.go['CGO_ENABLED'] = '0'
        with self.assertRaises(ValueError):
            self.collect()

    def test_compiler_wrapper_rejected(self):
        with self.assertRaisesRegex(ValueError, 'wrappers'):
            KEY.compiler_identity('ccache gcc')

    def test_missing_output_file_does_not_emit_a_cache_key(self):
        with mock.patch.dict(os.environ, {'GITHUB_OUTPUT': str(self.root / 'missing')}):
            self.assertEqual(KEY.main(), 1)
        self.assertFalse((self.root / 'missing').exists())

    def test_cache_upload_is_bounded_without_deleting_files(self):
        self.gocache.mkdir()
        file = self.gocache / 'entry'
        file.write_bytes(b'12345')
        paths = (str(self.gocache), str(self.modcache))
        self.assertEqual(SAVE.assess(paths, max_bytes=5, min_free=0), (True, 5, 'within upload bound'))
        self.assertEqual(SAVE.assess(paths, max_bytes=4, min_free=0), (False, 5, 'cache exceeds upload bound'))
        self.assertEqual(file.read_bytes(), b'12345')
        (self.gocache / 'link').symlink_to('/tmp')
        with self.assertRaisesRegex(ValueError, 'link or special'):
            SAVE.assess(paths, min_free=0)
        self.assertEqual(file.read_bytes(), b'12345')

    def test_unreadable_cache_subtree_is_not_reported_as_bounded(self):
        self.gocache.mkdir()

        def interrupted(root, *, followlinks=False, onerror=None):
            yield str(root), [], []
            if onerror is not None:
                onerror(PermissionError('unreadable cache subtree'))

        with mock.patch.object(SAVE.os, 'walk', side_effect=interrupted):
            with self.assertRaisesRegex(PermissionError, 'unreadable cache'):
                SAVE.assess((str(self.gocache), str(self.modcache)), min_free=0)

    def test_overlapping_or_unavailable_cache_paths_are_not_uploaded(self):
        self.gocache.mkdir()
        with self.assertRaisesRegex(ValueError, 'overlap'):
            SAVE.assess((str(self.gocache), str(self.gocache / 'child')), min_free=0)
        self.assertEqual(SAVE.assess((str(self.root / 'missing'), str(self.modcache)), min_free=0),
                         (False, 0, 'build cache is missing'))
        self.assertEqual(SAVE.assess((str(self.gocache), str(self.modcache)), min_free=10**30)[2],
                         'insufficient free storage')


if __name__ == '__main__':
    unittest.main()
