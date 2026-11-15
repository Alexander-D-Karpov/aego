import os
from pathlib import Path
import hashlib
import io
import json
import shutil
import stat
import subprocess
import sys
import tarfile
import tempfile
import unittest


class ProtocolInstallTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.home = self.root / 'home'
        self.data = self.root / 'data'
        self.home.mkdir()
        self.data.mkdir()
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        self.state = self.root / 'handler'
        xdg = self.bin / 'xdg-mime'
        xdg.write_text('''#!/bin/sh
case "$1:$2:$3" in
query:default:x-scheme-handler/aego) [ ! -f "$HANDLER_STATE" ] || cat "$HANDLER_STATE" ;;
default:*:x-scheme-handler/aego) printf '%s\\n' "$2" > "$HANDLER_STATE" ;;
esac
''')
        xdg.chmod(0o755)
        for name in ('update-mime-database', 'update-desktop-database'):
            helper = self.bin / name
            helper.write_text('#!/bin/sh\nexit 0\n')
            helper.chmod(0o755)
        script = (Path(__file__).resolve().parents[2] / 'tools/release/install.sh').read_text()
        start = script.index('install_desktop() {')
        end = script.index('\n}\n', start) + 2
        self.function = script[start:end]

    def install(self, prefix):
        env = dict(os.environ, HOME=str(self.home), XDG_DATA_HOME=str(self.data),
                   PATH=str(self.bin) + os.pathsep + os.defpath, HANDLER_STATE=str(self.state),
                   TEST_PREFIX=str(prefix))
        subprocess.run(['sh', '-c', 'set -eu\nprefix="$TEST_PREFIX"\nroot="$prefix/share/aego"\n' + self.function + '\ninstall_desktop\n'],
                       env=env, cwd=self.root, check=True, capture_output=True)

    @property
    def desktop(self):
        return self.data / 'applications/aego-example.desktop'

    def test_existing_default_and_modified_file_are_preserved(self):
        prefix = self.root / 'install with spaces'
        self.state.write_text('other.desktop\n')
        self.install(prefix)
        self.assertEqual(self.state.read_text(), 'other.desktop\n')
        self.assertIn('edit --example-link %u', self.desktop.read_text())
        modified = b'[Desktop Entry]\nName=User work\n'
        self.desktop.write_bytes(modified)
        self.install(prefix)
        self.assertEqual(self.desktop.read_bytes(), modified)
        self.assertEqual(self.state.read_text(), 'other.desktop\n')
        self.assertEqual(list(self.desktop.parent.glob('.aego-example.*')), [])

    def test_new_and_identical_owned_file_register_without_replacement(self):
        prefix = self.root / 'install with spaces'
        self.install(prefix)
        before = self.desktop.stat()
        self.assertEqual(self.state.read_text(), 'aego-example.desktop\n')
        self.install(prefix)
        self.assertEqual(self.desktop.stat().st_ino, before.st_ino)

    def test_symlink_and_target_are_untouched(self):
        self.desktop.parent.mkdir(parents=True)
        target = self.root / 'user-data'
        target.write_bytes(b'preserve')
        self.desktop.symlink_to(target)
        self.install(self.root / 'install')
        self.assertTrue(self.desktop.is_symlink())
        self.assertEqual(target.read_bytes(), b'preserve')
        self.assertFalse(self.state.exists())

    def test_unsafe_exec_prefix_does_not_register_protocol(self):
        for name in ('literal%U', 'quote"path', 'line\nbreak', 'tab\tpath', 'dollar$path'):
            with self.subTest(prefix=name):
                self.install(self.root / name)
                self.assertFalse(self.desktop.exists())
                self.assertFalse((self.data / 'applications/aego.desktop').exists())
                self.assertFalse(self.state.exists())


@unittest.skipUnless(sys.platform.startswith('linux'), 'the tarball installer targets Linux')
class InstallerArchiveTests(unittest.TestCase):
    """Run the real installer against isolated, checksum-verified fake releases."""

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.fixture = self.root / 'fixture'
        self.bin = self.root / 'bin'
        self.home = self.root / 'home'
        self.work = self.root / 'tmp'
        for path in (self.fixture, self.bin, self.home, self.work):
            path.mkdir()
        curl = self.bin / 'curl'
        curl.write_text('''#!/bin/sh
out= url=
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) out=$2; shift 2 ;;
    http*) url=$1; shift ;;
    *) shift ;;
  esac
done
case "$url" in
  */releases/*) cat "$MOCK_FIXTURE/api.json" ;;
  */SHA256SUMS.txt*) cp "$MOCK_FIXTURE/SHA256SUMS.txt" "$out" ;;
  *-linux-amd64.tar.gz*) cp "$MOCK_FIXTURE/aego-test-linux-amd64.tar.gz" "$out" ;;
  *) exit 22 ;;
esac
''')
        curl.chmod(0o755)
        uname = self.bin / 'uname'
        uname.write_text('#!/bin/sh\ncase "$1" in -s) echo Linux ;; -m) echo x86_64 ;; *) exit 1 ;; esac\n')
        uname.chmod(0o755)
        self.script = Path(__file__).resolve().parents[2] / 'tools/release/install.sh'

    def install(self, prefix, version=b'v0.12.5\n' + b'a' * 40 + b'\n',
                version_link=None, extra_top=False, binary=True, checksum=True, env_overrides=None):
        archive = self.fixture / 'aego-test-linux-amd64.tar.gz'
        with tarfile.open(archive, 'w:gz') as tar:
            if version_link is not None:
                info = tarfile.TarInfo('aego-fixture/VERSION')
                info.type = tarfile.SYMTYPE
                info.linkname = version_link
                tar.addfile(info)
            elif version is not None:
                info = tarfile.TarInfo('aego-fixture/VERSION')
                info.mode = 0o644
                info.size = len(version)
                tar.addfile(info, io.BytesIO(version))
            if binary:
                data = b'#!/bin/sh\nexit 0\n'
                info = tarfile.TarInfo('aego-fixture/bin/aego')
                info.mode = 0o755
                info.size = len(data)
                tar.addfile(info, io.BytesIO(data))
            if extra_top:
                info = tarfile.TarInfo('aego-other/extra')
                info.size = 1
                tar.addfile(info, io.BytesIO(b'x'))
        digest = hashlib.sha256(archive.read_bytes()).hexdigest() if checksum else '0' * 64
        (self.fixture / 'SHA256SUMS.txt').write_text(digest + '  ' + archive.name + '\n')
        (self.fixture / 'api.json').write_text(json.dumps({'assets': [
            {'browser_download_url': 'https://fixture.invalid/' + archive.name},
            {'browser_download_url': 'https://fixture.invalid/SHA256SUMS.txt'},
        ]}, separators=(',', ':')))
        env = dict(os.environ, HOME=str(self.home), TMPDIR=str(self.work),
                   PATH=str(self.bin) + os.pathsep + os.defpath,
                   MOCK_FIXTURE=str(self.fixture), AEGO_API='https://fixture.invalid/api')
        env.update(env_overrides or {})
        return subprocess.run(['sh', str(self.script), '--prefix', str(prefix),
                               '--no-deps', '--no-desktop'], cwd=self.root, env=env,
                              capture_output=True, text=True, timeout=30)

    def populated_prefix(self, name):
        prefix = self.root / name
        installed = prefix / 'share/aego'
        (installed / '0.11.0').mkdir(parents=True)
        (installed / '0.11.0/note.txt').write_bytes(b'installed user data')
        (prefix / 'share/victim').mkdir()
        (prefix / 'share/victim/note.txt').write_bytes(b'sibling user data')
        (installed / 'current').symlink_to('0.11.0')
        return prefix

    def snapshot(self, path):
        result = {}
        for base, dirs, files in os.walk(path, followlinks=False):
            for name in dirs + files:
                item = Path(base) / name
                key = str(item.relative_to(path))
                if item.is_symlink():
                    result[key] = ('link', os.readlink(item))
                elif item.is_dir():
                    result[key] = ('directory',)
                else:
                    result[key] = ('file', item.read_bytes())
        return result

    def test_invalid_version_preserves_the_entire_prefix(self):
        bad = [b'', b'\n', b'../victim\n', b'v../victim\n', b'/absolute\n',
               b'.\n', b'..\n', b'current\n', b'v\n', b'v0.12.5/../../victim\n',
               b'v0.12.5\\escape\n', b'v0.12.5\r\n', b'nightly\n',
               b'nightly\n../victim\n', b'nightly\n1234567\n']
        for index, version in enumerate(bad):
            with self.subTest(version=version):
                prefix = self.populated_prefix('bad-' + str(index))
                before = self.snapshot(prefix)
                result = self.install(prefix, version)
                self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertEqual(self.snapshot(prefix), before)

    def test_invalid_archive_preserves_the_entire_prefix(self):
        cases = [dict(version=None), dict(version_link='/etc/passwd'),
                 dict(extra_top=True), dict(binary=False), dict(checksum=False)]
        for index, options in enumerate(cases):
            with self.subTest(options=options):
                prefix = self.populated_prefix('archive-' + str(index))
                before = self.snapshot(prefix)
                result = self.install(prefix, **options)
                self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertEqual(self.snapshot(prefix), before)

    def test_fresh_install_and_reinstall_keep_the_previous_contents(self):
        prefix = self.root / 'install with spaces'
        first = self.install(prefix)
        self.assertEqual(first.returncode, 0, first.stdout + first.stderr)
        installed = prefix / 'share/aego'
        old = os.readlink(installed / 'current')
        self.assertEqual(old, '0.12.5')
        (installed / old / 'user-note').write_bytes(b'keep this')
        second = self.install(prefix)
        self.assertEqual(second.returncode, 0, second.stdout + second.stderr)
        current = os.readlink(installed / 'current')
        self.assertNotEqual(current, old)
        self.assertNotIn('/', current)
        self.assertEqual(os.readlink(installed / 'previous'), old)
        self.assertEqual((installed / old / 'user-note').read_bytes(), b'keep this')
        self.assertTrue(os.access(installed / current / 'bin/aego', os.X_OK))
        self.assertEqual((prefix / 'bin/aego').resolve(), (installed / current / 'bin/aego').resolve())

    def test_occupied_version_symlink_and_target_are_preserved(self):
        prefix = self.populated_prefix('version-symlink')
        installed = prefix / 'share/aego'
        outside = self.root / 'external-version'
        outside.mkdir()
        (outside / 'note').write_bytes(b'external data')
        (installed / '0.12.5').symlink_to(outside)
        result = self.install(prefix)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue((installed / '0.12.5').is_symlink())
        self.assertEqual((outside / 'note').read_bytes(), b'external data')
        self.assertNotEqual(os.readlink(installed / 'current'), '0.12.5')
        self.assertEqual(os.readlink(installed / 'previous'), '0.11.0')

    def test_failed_copy_does_not_switch_existing_installation(self):
        prefix = self.populated_prefix('copy-failure')
        copy = self.bin / 'cp'
        copy.write_text('''#!/bin/sh
if [ "$1" = -a ]; then exit 42; fi
exec /bin/cp "$@"
''')
        copy.chmod(0o755)
        installed = prefix / 'share/aego'
        result = self.install(prefix)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(os.readlink(installed / 'current'), '0.11.0')
        self.assertEqual((installed / '0.11.0/note.txt').read_bytes(), b'installed user data')
        self.assertEqual((prefix / 'share/victim/note.txt').read_bytes(), b'sibling user data')
        self.assertFalse((prefix / 'bin/aego').exists())

    def test_nightly_and_legacy_install(self):
        prefix = self.root / 'legacy'
        installed = prefix / 'share/aego'
        (installed / 'bin').mkdir(parents=True)
        (installed / 'bin/aego').write_bytes(b'legacy binary')
        (prefix / 'bin').mkdir()
        (prefix / 'bin/aego').symlink_to(installed / 'bin/aego')
        result = self.install(prefix, b'nightly\n' + b'abcdef0' + b'1' * 33 + b'\n')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(os.readlink(installed / 'current'), 'nightly-abcdef0')
        self.assertEqual(os.readlink(installed / 'previous'), 'legacy')
        self.assertEqual((installed / 'legacy/bin/aego').read_bytes(), b'legacy binary')

    def test_existing_legacy_backup_is_not_removed(self):
        prefix = self.root / 'legacy-conflict'
        installed = prefix / 'share/aego'
        (installed / 'bin').mkdir(parents=True)
        (installed / 'bin/aego').write_bytes(b'legacy binary')
        backup = prefix / 'share/aego.legacy'
        backup.mkdir()
        (backup / 'note').write_bytes(b'unrelated data')
        before = self.snapshot(prefix)
        result = self.install(prefix)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.snapshot(prefix), before)

    def test_failed_legacy_moves_do_not_discard_old_binary(self):
        for index, fail_last in enumerate((False, True)):
            with self.subTest(fail_last=fail_last):
                prefix = self.root / ('legacy-move-' + str(index))
                installed = prefix / 'share/aego'
                (installed / 'bin').mkdir(parents=True)
                (installed / 'bin/aego').write_bytes(b'legacy binary')
                (prefix / 'bin').mkdir()
                move = self.bin / 'mv'
                move.write_text('''#!/bin/sh
case "$1" in
  *.legacy) [ "$MOCK_FAIL_LAST" = 1 ] && exit 41 ;;
  *) [ "$MOCK_FAIL_LAST" = 0 ] && exit 41 ;;
esac
exec /bin/mv "$@"
''')
                move.chmod(0o755)
                result = self.install(prefix, env_overrides={'MOCK_FAIL_LAST': '1' if fail_last else '0'})
                self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
                preserved = prefix / 'share/aego.legacy' if fail_last else installed
                self.assertEqual((preserved / 'bin/aego').read_bytes(), b'legacy binary')
                self.assertFalse((installed / 'current').exists())
                if fail_last:
                    self.assertIn(str(preserved), result.stderr)

    def test_existing_non_link_entry_points_are_preserved(self):
        for index, name in enumerate(('share/aego/current', 'share/aego/previous', 'bin/aego')):
            with self.subTest(name=name):
                prefix = self.root / ('entry-' + str(index))
                target = prefix / name
                target.parent.mkdir(parents=True)
                target.write_bytes(b'unrelated data')
                before = self.snapshot(prefix)
                result = self.install(prefix)
                self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertEqual(self.snapshot(prefix), before)

    def test_installed_root_symlink_is_not_followed(self):
        prefix = self.root / 'linked-root'
        outside = self.root / 'outside'
        outside.mkdir()
        (outside / 'note').write_bytes(b'unrelated data')
        (prefix / 'share').mkdir(parents=True)
        (prefix / 'share/aego').symlink_to(outside)
        before = self.snapshot(prefix)
        result = self.install(prefix)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.snapshot(prefix), before)
        self.assertEqual((outside / 'note').read_bytes(), b'unrelated data')

    def test_existing_unrelated_command_link_is_preserved(self):
        prefix = self.populated_prefix('other-command')
        (prefix / 'bin').mkdir()
        (prefix / 'bin/aego').symlink_to('/opt/other/aego')
        before = self.snapshot(prefix)
        result = self.install(prefix)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.snapshot(prefix), before)


@unittest.skipUnless(sys.platform.startswith('linux'), 'AppImage builds target Linux')
class AppImageToolTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.repo = self.root / 'repo'
        self.script = self.repo / 'tools/release/appimage.sh'
        self.script.parent.mkdir(parents=True)
        self.script.write_bytes((Path(__file__).resolve().parents[2] / 'tools/release/appimage.sh').read_bytes())
        icon = self.repo / 'app/brand/png/aego-256.png'
        icon.parent.mkdir(parents=True)
        icon.write_bytes(b'synthetic icon')
        self.src = self.root / 'stage'
        (self.src / 'bin').mkdir(parents=True)
        (self.src / 'bin/aego').write_bytes(b'synthetic binary')
        (self.src / 'VERSION').write_text('v0.12.5\n')
        (self.src / 'README.txt').write_text('synthetic readme\n')
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        for name in ('dirname', 'mkdir', 'mktemp', 'chmod', 'cp', 'sha256sum', 'cat', 'rm', 'readlink', 'wc'):
            tool = shutil.which(name)
            if tool is None:
                self.skipTest(name + ' is unavailable')
            (self.bin / name).symlink_to(tool)
        curl = self.bin / 'curl'
        curl.write_text('''#!/bin/sh
out= url=
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) out=$2; shift 2 ;;
    http*) url=$1; shift ;;
    *) shift ;;
  esac
done
case "$url" in
  */runtime-x86_64) /bin/cp "$MOCK_RUNTIME" "$out" ;;
  */appimagetool-x86_64.AppImage) /bin/cp "$MOCK_DOWNLOAD" "$out" ;;
  *) exit 1 ;;
esac
''')
        curl.chmod(0o755)
        self.download = self.root / 'download'
        self.download.write_bytes(b'#!/bin/sh\n[ "$2" = --runtime-file ] || exit 4\n'
                                  b'[ "$(cat "$3")" = "synthetic runtime" ] || exit 5\n'
                                  b'printf executed > "$MOCK_EXECUTED"\nprintf output > "$5"\n')
        self.runtime = self.root / 'runtime'
        self.runtime.write_bytes(b'synthetic runtime')
        self.executed = self.root / 'executed'
        self.out = self.root / 'output.AppImage'
        self.expected = hashlib.sha256(self.download.read_bytes()).hexdigest()
        self.runtime_expected = hashlib.sha256(self.runtime.read_bytes()).hexdigest()

    def run_builder(self, expected=None, **extra):
        env = dict(os.environ, PATH=str(self.bin), HOME=str(self.root), TMPDIR=str(self.root),
                   MOCK_DOWNLOAD=str(self.download), MOCK_RUNTIME=str(self.runtime), MOCK_EXECUTED=str(self.executed),
                   APPIMAGETOOL='', APPIMAGETOOL_SHA256=expected or self.expected,
                   APPIMAGE_RUNTIME='', APPIMAGE_RUNTIME_SHA256=self.runtime_expected)
        env.update(extra)
        return subprocess.run(['/bin/sh', str(self.script), str(self.src), str(self.out)],
                              cwd=self.root, env=env, capture_output=True, text=True, timeout=30)

    def test_download_must_match_the_expected_checksum_before_execution(self):
        result = self.run_builder(expected='0' * 64)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse(self.executed.exists())
        self.assertFalse(self.out.exists())

    def test_matching_download_is_executed(self):
        result = self.run_builder()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.executed.read_text(), 'executed')
        self.assertEqual(self.out.read_text(), 'output')

    def test_cached_tool_is_checked_and_not_replaced(self):
        tool = self.repo / '.cache/tools/appimagetool'
        tool.parent.mkdir(parents=True)
        bad = b'#!/bin/sh\nprintf cached > "$MOCK_EXECUTED"\n'
        tool.write_bytes(bad)
        tool.chmod(0o755)
        result = self.run_builder()
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse(self.executed.exists())
        self.assertEqual(tool.read_bytes(), bad)

    def test_matching_cached_tool_is_reused(self):
        tool = self.repo / '.cache/tools/appimagetool'
        tool.parent.mkdir(parents=True)
        tool.write_bytes(self.download.read_bytes())
        tool.chmod(0o755)
        self.download.unlink()
        result = self.run_builder()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.executed.read_text(), 'executed')

    def test_cached_symlink_is_neither_followed_nor_replaced(self):
        tool = self.repo / '.cache/tools/appimagetool'
        tool.parent.mkdir(parents=True)
        tool.symlink_to(self.download)
        result = self.run_builder()
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue(tool.is_symlink())
        self.assertFalse(self.executed.exists())

    def test_explicit_local_tool_remains_caller_managed(self):
        self.download.chmod(0o755)
        result = self.run_builder(expected='bad', APPIMAGETOOL=str(self.download))
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.executed.read_text(), 'executed')

    def test_bad_checksum_configuration_fails_without_execution(self):
        result = self.run_builder(expected='bad')
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse(self.executed.exists())

    def test_runtime_must_match_before_the_tool_runs(self):
        result = self.run_builder(APPIMAGE_RUNTIME_SHA256='0' * 64)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse(self.executed.exists())
        self.assertFalse(self.out.exists())

    def test_explicit_runtime_is_checked_and_passed_to_tool(self):
        result = self.run_builder(APPIMAGE_RUNTIME=str(self.runtime))
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.executed.read_text(), 'executed')


@unittest.skipUnless(sys.platform.startswith('linux'), 'bootstrap path fixtures use POSIX tools')
class NativeBootstrapPathTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)

    def fixture(self, name):
        repo = self.root / name
        script = repo / 'tools/bootstrap-native.sh'
        script.parent.mkdir(parents=True)
        script.write_bytes((Path(__file__).resolve().parents[2] / 'tools/bootstrap-native.sh').read_bytes())
        archive = repo / '.cache/download/SDL3-3.2.24.tar.gz'
        archive.parent.mkdir(parents=True, exist_ok=True)
        with tarfile.open(archive, 'w:gz') as tar:
            info = tarfile.TarInfo('SDL3-fixture/README')
            info.size = 1
            tar.addfile(info, io.BytesIO(b'x'))
        version = repo / 'third_party/sdl/version.txt'
        version.parent.mkdir(parents=True)
        version.write_text('version=3.2.24\nsha256=' + hashlib.sha256(archive.read_bytes()).hexdigest() + '\n')
        bindir = repo / 'bin'
        bindir.mkdir()
        cmake = bindir / 'cmake'
        cmake.write_text('''#!/bin/sh
printf '%s\\n' "$*" >> "$MOCK_CMAKE_CALLS"
while [ "$#" -gt 0 ]; do
  if [ "$1" = --prefix ]; then mkdir -p "$2"; break; fi
  shift
done
''')
        cmake.chmod(0o755)
        return repo, script, bindir

    def run_bootstrap(self, repo, script, bindir, goos, goarch):
        env = dict(os.environ, PATH=str(bindir) + os.pathsep + os.defpath,
                   HOME=str(repo), GOOS=goos, GOARCH=goarch, FORCE='1',
                   MOCK_CMAKE_CALLS=str(repo / 'cmake-calls'))
        return subprocess.run(['bash', str(script)], cwd=repo, env=env,
                              capture_output=True, text=True, timeout=30)

    def test_invalid_targets_cannot_remove_other_directories(self):
        for index, (goos, goarch) in enumerate((('x/../../../victim', 'amd64'),
                                                ('linux', 'x/../../../victim'),
                                                ('linux', 'bad arch'))):
            with self.subTest(goos=goos, goarch=goarch):
                repo, script, bindir = self.fixture('invalid-' + str(index))
                work = repo / '.cache/build' / ('sdl-3.2.24-' + goos + '_' + goarch)
                sentinel = work / 'src/user-data'
                sentinel.parent.mkdir(parents=True)
                self.assertTrue(sentinel.resolve().is_relative_to(repo.resolve()))
                sentinel.write_bytes(b'preserve existing data')
                result = self.run_bootstrap(repo, script, bindir, goos, goarch)
                self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertTrue(sentinel.exists(), 'bootstrap removed the pre-existing source directory')
                self.assertEqual(sentinel.read_bytes(), b'preserve existing data')
                self.assertFalse((repo / 'cmake-calls').exists())

    def test_supported_targets_reach_the_fake_builder(self):
        targets = [('linux', 'amd64'), ('linux', 'arm64'), ('darwin', 'amd64'),
                   ('darwin', 'arm64'), ('windows', 'amd64')]
        for index, (goos, goarch) in enumerate(targets):
            with self.subTest(goos=goos, goarch=goarch):
                repo, script, bindir = self.fixture('valid-' + str(index))
                result = self.run_bootstrap(repo, script, bindir, goos, goarch)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertTrue((repo / '.cache/native' / (goos + '_' + goarch) / '.sdl-3.2.24').is_file())
                self.assertEqual(len((repo / 'cmake-calls').read_text().splitlines()), 3)

    def test_symlinked_build_root_is_not_followed(self):
        repo, script, bindir = self.fixture('linked-build')
        work = repo / '.cache/build/sdl-3.2.24-linux_amd64'
        work.parent.mkdir(parents=True)
        outside = repo / 'unrelated-data'
        (outside / 'src').mkdir(parents=True)
        sentinel = outside / 'src/note'
        sentinel.write_bytes(b'preserve user data')
        work.symlink_to(outside, target_is_directory=True)
        result = self.run_bootstrap(repo, script, bindir, 'linux', 'amd64')
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue(work.is_symlink())
        self.assertEqual(sentinel.read_bytes(), b'preserve user data')

    def test_download_partial_symlink_is_not_followed(self):
        repo, script, bindir = self.fixture('linked-download')
        archive = repo / '.cache/download/SDL3-3.2.24.tar.gz'
        archive.unlink()
        target = repo / 'unrelated-download'
        target.write_bytes(b'preserve user data')
        partial = repo / '.cache/download/SDL3-3.2.24.tar.gz.part'
        partial.symlink_to(target)
        curl = bindir / 'curl'
        curl.write_text('''#!/bin/sh
while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ]; then printf overwritten > "$2"; exit; fi
  shift
done
exit 1
''')
        curl.chmod(0o755)
        result = self.run_bootstrap(repo, script, bindir, 'linux', 'amd64')
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(target.read_bytes(), b'preserve user data')
        self.assertTrue(partial.is_symlink())

    def test_existing_download_partial_is_not_overwritten(self):
        repo, script, bindir = self.fixture('occupied-partial')
        archive = repo / '.cache/download/SDL3-3.2.24.tar.gz'
        archive.unlink()
        partial = repo / '.cache/download/SDL3-3.2.24.tar.gz.part'
        partial.write_bytes(b'preserve existing partial')
        curl = bindir / 'curl'
        curl.write_text('''#!/bin/sh
printf called > "$HOME/curl-called"
while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ]; then printf overwritten > "$2"; exit; fi
  shift
done
exit 1
''')
        curl.chmod(0o755)
        result = self.run_bootstrap(repo, script, bindir, 'linux', 'amd64')
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue(partial.is_file(), 'bootstrap removed an existing partial')
        self.assertEqual(partial.read_bytes(), b'preserve existing partial')
        self.assertFalse((repo / 'curl-called').exists())
        self.assertFalse(archive.exists())

    def test_new_download_is_reserved_and_installed(self):
        repo, script, bindir = self.fixture('new-download')
        archive = repo / '.cache/download/SDL3-3.2.24.tar.gz'
        expected = archive.read_bytes()
        (repo / 'synthetic-download.tar.gz').write_bytes(expected)
        archive.unlink()
        curl = bindir / 'curl'
        curl.write_text('''#!/bin/sh
while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ]; then exec /bin/cp "$HOME/synthetic-download.tar.gz" "$2"; fi
  shift
done
exit 1
''')
        curl.chmod(0o755)
        result = self.run_bootstrap(repo, script, bindir, 'linux', 'amd64')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(archive.read_bytes(), expected)
        self.assertFalse(Path(str(archive) + '.part').exists())
        self.assertEqual(len((repo / 'cmake-calls').read_text().splitlines()), 3)

    def test_existing_fifo_partial_is_not_opened_or_replaced(self):
        repo, script, bindir = self.fixture('fifo-partial')
        archive = repo / '.cache/download/SDL3-3.2.24.tar.gz'
        archive.unlink()
        partial = Path(str(archive) + '.part')
        os.mkfifo(partial)
        curl = bindir / 'curl'
        curl.write_text('#!/bin/sh\nprintf called > "$HOME/curl-called"\nexit 16\n')
        curl.chmod(0o755)
        # Avoid hanging a vulnerable implementation on the FIFO open.
        reader = os.open(partial, os.O_RDONLY | os.O_NONBLOCK)
        try:
            result = self.run_bootstrap(repo, script, bindir, 'linux', 'amd64')
        finally:
            os.close(reader)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue(stat.S_ISFIFO(partial.lstat().st_mode))
        self.assertFalse((repo / 'curl-called').exists())

    def test_archive_appearing_during_download_is_preserved(self):
        repo, script, bindir = self.fixture('concurrent-download')
        archive = repo / '.cache/download/SDL3-3.2.24.tar.gz'
        expected = archive.read_bytes()
        (repo / 'synthetic-download.tar.gz').write_bytes(expected)
        archive.unlink()
        curl = bindir / 'curl'
        curl.write_text('''#!/bin/sh
printf 'another process archive' > "$HOME/.cache/download/SDL3-3.2.24.tar.gz"
while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ]; then exec /bin/cp "$HOME/synthetic-download.tar.gz" "$2"; fi
  shift
done
exit 1
''')
        curl.chmod(0o755)
        result = self.run_bootstrap(repo, script, bindir, 'linux', 'amd64')
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(archive.read_bytes(), b'another process archive')
        self.assertEqual(Path(str(archive) + '.part').read_bytes(), expected)
        self.assertFalse((repo / 'cmake-calls').exists())

    def test_cached_archive_with_wrong_checksum_is_preserved(self):
        repo, script, bindir = self.fixture('wrong-checksum')
        archive = repo / '.cache/download/SDL3-3.2.24.tar.gz'
        archive.write_bytes(b'preserve unexpected cached contents')
        result = self.run_bootstrap(repo, script, bindir, 'linux', 'amd64')
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue(archive.is_file(), 'bootstrap deleted the unexpected cached archive')
        self.assertEqual(archive.read_bytes(), b'preserve unexpected cached contents')
        self.assertFalse((repo / 'cmake-calls').exists())

    def test_archive_path_directory_is_not_used_as_download_target(self):
        repo, script, bindir = self.fixture('archive-directory')
        archive = repo / '.cache/download/SDL3-3.2.24.tar.gz'
        archive.unlink()
        archive.mkdir()
        (archive / 'user-note').write_bytes(b'preserve directory contents')
        curl = bindir / 'curl'
        curl.write_text('''#!/bin/sh
printf called > "$HOME/curl-called"
while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ]; then printf downloaded > "$2"; exit; fi
  shift
done
exit 1
''')
        curl.chmod(0o755)
        result = self.run_bootstrap(repo, script, bindir, 'linux', 'amd64')
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual((archive / 'user-note').read_bytes(), b'preserve directory contents')
        self.assertEqual(sorted(p.name for p in archive.iterdir()), ['user-note'])
        self.assertFalse((repo / 'curl-called').exists())

    def test_version_path_is_rejected_before_cleanup(self):
        repo, script, bindir = self.fixture('bad-version')
        original = repo / '.cache/download/SDL3-3.2.24.tar.gz'
        archive = repo / '.cache/download/SDL3-3.2.24/../bad.tar.gz'
        (repo / '.cache/download/SDL3-3.2.24').mkdir()
        archive.write_bytes(original.read_bytes())
        (repo / 'third_party/sdl/version.txt').write_text(
            'version=3.2.24/../bad\nsha256=' + hashlib.sha256(archive.read_bytes()).hexdigest() + '\n')
        work = repo / '.cache/build/sdl-3.2.24/../bad-linux_amd64'
        sentinel = work / 'src/user-data'
        sentinel.parent.mkdir(parents=True, exist_ok=True)
        sentinel.write_bytes(b'preserve existing data')
        result = self.run_bootstrap(repo, script, bindir, 'linux', 'amd64')
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue(sentinel.exists(), 'bootstrap removed a source directory for a malformed version')
        self.assertEqual(sentinel.read_bytes(), b'preserve existing data')


class AppImageArgumentTests(unittest.TestCase):
    def test_wrapper_preserves_url_as_one_restricted_argument(self):
        # Exercise AppRun without building an AppImage or touching installed apps.
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            script = (Path(__file__).resolve().parents[2] / 'tools/release/appimage.sh').read_text()
            begin = script.index('cat > "$app/AppRun" <<\'RUN\'\n') + len('cat > "$app/AppRun" <<\'RUN\'\n')
            end = script.index('\nRUN\n', begin)
            wrapper = root / 'AppRun'
            wrapper.write_text(script[begin:end] + '\n')
            wrapper.chmod(0o755)
            binary = root / 'usr/bin/aego'
            binary.parent.mkdir(parents=True)
            binary.write_text(f'#!{sys.executable}\nimport json,sys\nprint(json.dumps(sys.argv[1:]))\n')
            binary.chmod(0o755)
            for incoming, expected in [
                ([], ['edit']),
                (['/tmp/project.aego'], ['edit', '/tmp/project.aego']),
                (['file:///tmp/My%20Game/project.aego'], ['edit', '--file-link', 'file:///tmp/My%20Game/project.aego']),
                (['aego://example?manifest=https%3A%2F%2Fexample.invalid%2Fx'],
                 ['edit', '--example-link', 'aego://example?manifest=https%3A%2F%2Fexample.invalid%2Fx']),
                (['aego://example?x=y', '--tool'], ['edit', '--example-link', 'aego://example?x=y', '--tool']),
            ]:
                result = subprocess.run([str(wrapper), *incoming], capture_output=True, text=True, check=True)
                self.assertEqual(json.loads(result.stdout), expected)


if __name__ == '__main__':
    unittest.main()
