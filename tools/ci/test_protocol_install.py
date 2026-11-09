import os
from pathlib import Path
import json
import subprocess
import sys
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


class AppImageArgumentTests(unittest.TestCase):
    def test_wrapper_preserves_url_as_one_restricted_argument(self):
        # Exercise the shipped AppRun script without creating an AppImage,
        # invoking appimagetool or touching an installed user application.
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
