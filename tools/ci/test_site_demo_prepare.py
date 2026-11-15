import importlib.util
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('site_demo_prepare', ROOT / 'tools/site_demo_prepare.py')
prepare = importlib.util.module_from_spec(spec)
spec.loader.exec_module(prepare)


class SiteDemoPrepareTests(unittest.TestCase):
    def test_only_staged_scene_changes_and_original_pit_remains(self):
        original = (ROOT / 'examples/platformer/scenes/main.aescene').read_bytes()
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            source = root / 'source'
            (source / 'scenes').mkdir(parents=True)
            (source / 'scenes/main.aescene').write_bytes(original)
            (source / 'scenes/main.aescene.meta').write_bytes(b'original metadata\n')
            (source / '.aego').mkdir()
            (source / '.aego/keep').write_bytes(b'private\n')
            dest = root / 'demo'
            prepare.prepare(source, dest)
            self.assertEqual((source / 'scenes/main.aescene').read_bytes(), original)
            self.assertEqual((dest / 'scenes/main.aescene.meta').read_bytes(), b'original metadata\n')
            self.assertFalse((dest / '.aego').exists())
            result = (dest / 'scenes/main.aescene').read_text()
            self.assertEqual(result, prepare.close_checkpoint_gap(original.decode()))
            self.assertIn('points=[[-7, -14], [-7, -0.5], [7, -0.5], [7, 1.5]]', result)
            self.assertIn('position=[21.5, 4]', result)

    def test_changed_ground_fails_closed(self):
        scene = (ROOT / 'examples/platformer/scenes/main.aescene').read_text()
        with self.assertRaises(ValueError):
            prepare.close_checkpoint_gap(scene.replace('size=[10, 1]', 'size=[9, 1]', 1))
        with self.assertRaises(ValueError):
            prepare.close_checkpoint_gap(prepare.close_checkpoint_gap(scene))

    def test_symlink_and_existing_destination_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            source = root / 'source'
            source.mkdir()
            (source / 'link').symlink_to(root)
            with self.assertRaises(ValueError):
                prepare.prepare(source, root / 'demo')
            with self.assertRaises(ValueError):
                prepare.prepare(source, source)


class SiteDemoPublicationTests(unittest.TestCase):
    """Exercise publication and cleanup with disposable output and fake mv."""

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        exporter = self.root / 'exporter'
        exporter.write_text('''#!/bin/sh
set -eu
[ "$1" = --out ]
mkdir "$2"
printf '%s\n' '<html lang="en"><body>new demo</body></html>' > "$2/index.html"
''')
        exporter.chmod(0o755)
        self.exporter = exporter
        move = self.bin / 'mv'
        move.write_text('''#!/bin/sh
case "$1" in
  */new)
    case "$TEST_PUBLISH" in
      fail) exit 41 ;;
      occupied) mkdir "$2"; printf '%s\\n' concurrent > "$2/other"; exit 41 ;;
      occupied-move) mkdir "$2"; printf '%s\\n' concurrent > "$2/other" ;;
    esac ;;
  */previous) [ "$TEST_ROLLBACK" != fail ] || exit 42 ;;
esac
exec /bin/mv "$@"
''')
        move.chmod(0o755)

    def publish(self, out, publish='ok', rollback='ok'):
        env = dict(os.environ, PATH=str(self.bin) + os.pathsep + os.defpath,
                   TEST_PUBLISH=publish, TEST_ROLLBACK=rollback)
        return subprocess.run(['sh', str(ROOT / 'tools/site-demo.sh'), str(out), str(self.exporter)],
                              cwd=self.root, env=env, capture_output=True, text=True, timeout=30)

    def previous(self):
        return list(self.root.glob('.site-demo.*/previous'))

    def test_failed_publish_restores_previous_output(self):
        out = self.root / 'demo'
        out.mkdir()
        (out / 'old').write_bytes(b'previous demo')
        result = self.publish(out, publish='fail')
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual((out / 'old').read_bytes(), b'previous demo')
        self.assertEqual(self.previous(), [])

    def test_successful_publish_replaces_the_generated_output(self):
        out = self.root / 'demo'
        out.mkdir()
        (out / 'old').write_bytes(b'previous demo')
        result = self.publish(out)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn('data-sound="off"', (out / 'index.html').read_text())
        self.assertTrue((out / 'index.html.gz').is_file())
        self.assertEqual(self.previous(), [])

    def test_concurrent_destination_keeps_both_outputs(self):
        out = self.root / 'demo'
        out.mkdir()
        (out / 'old').write_bytes(b'previous demo')
        result = self.publish(out, publish='occupied')
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual((out / 'other').read_text(), 'concurrent\n')
        previous = self.previous()
        self.assertEqual(len(previous), 1, result.stderr)
        self.assertEqual((previous[0] / 'old').read_bytes(), b'previous demo')
        self.assertIn(str(previous[0]), result.stderr)

    def test_failed_rollback_retains_and_reports_previous_output(self):
        out = self.root / 'demo'
        out.mkdir()
        (out / 'old').write_bytes(b'previous demo')
        result = self.publish(out, publish='fail', rollback='fail')
        self.assertNotEqual(result.returncode, 0)
        previous = self.previous()
        self.assertEqual(len(previous), 1, result.stderr)
        self.assertEqual((previous[0] / 'old').read_bytes(), b'previous demo')
        self.assertIn(str(previous[0]), result.stderr)

    def test_directory_appearing_during_move_does_not_discard_previous(self):
        out = self.root / 'demo'
        out.mkdir()
        (out / 'old').write_bytes(b'previous demo')
        result = self.publish(out, publish='occupied-move')
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual((out / 'other').read_text(), 'concurrent\n')
        previous = self.previous()
        self.assertEqual(len(previous), 1, result.stderr)
        self.assertEqual((previous[0] / 'old').read_bytes(), b'previous demo')
        self.assertIn(str(previous[0]), result.stderr)

    def test_existing_file_or_symlink_is_not_replaced(self):
        for index, kind in enumerate(('file', 'directory-symlink', 'dangling-symlink')):
            with self.subTest(kind=kind):
                out = self.root / ('demo-' + str(index))
                if kind == 'file':
                    out.write_bytes(b'unrelated file')
                else:
                    target = self.root / ('target-' + str(index))
                    if kind == 'directory-symlink':
                        target.mkdir()
                        (target / 'note').write_bytes(b'unrelated directory')
                    out.symlink_to(target)
                result = self.publish(out)
                self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
                if kind == 'file':
                    self.assertEqual(out.read_bytes(), b'unrelated file')
                else:
                    self.assertTrue(out.is_symlink())
                    self.assertEqual(os.readlink(out), str(target))
                    if kind == 'directory-symlink':
                        self.assertEqual((target / 'note').read_bytes(), b'unrelated directory')


if __name__ == '__main__':
    unittest.main()
