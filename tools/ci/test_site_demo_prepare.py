import importlib.util
from pathlib import Path
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


if __name__ == '__main__':
    unittest.main()
