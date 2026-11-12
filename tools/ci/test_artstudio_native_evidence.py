import hashlib
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]


class NativeStudioEvidenceTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='aego-native-evidence-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.root.joinpath('required-tests.txt').write_text('TestArtSceneAuthoringWorkflowNative\n')
        events = [{'Test': 'TestArtSceneAuthoringWorkflowNative', 'Action': action} for action in ('run', 'pass')]
        self.root.joinpath('events.jsonl').write_text(''.join(json.dumps(event) + '\n' for event in events))
        snapshot = json.dumps({'worktreeSHA256': 'test-snapshot', 'head': 'test-commit'})
        self.root.joinpath('source-before.json').write_text(snapshot)
        self.root.joinpath('source-after.json').write_text(snapshot)
        self.root.joinpath('tests.exit').write_text('0\n')

    def producer(self):
        root = self.root / 'interoperability'
        project = root / 'project'
        files = {}
        for name, data in {
            'assets/hero.aeart': b'synthetic drawing',
            'assets/hero.aeart.meta': b'source metadata',
            'scenes/art-workflow.aescene': b'scene',
            'scenes/art-workflow.aescene.meta': b'scene metadata',
            'project.aego': b'project',
        }.items():
            target = project / name
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(data)
            files[name] = hashlib.sha256(data).hexdigest()
        source = project / 'assets/hero.aeart'
        sha = files['assets/hero.aeart']
        root.joinpath('native-source.png').write_bytes(b'\x89PNG\r\n\x1a\nsynthetic')
        witness = {
            'schema': 1,
            'sourcePath': 'assets/hero.aeart',
            'scenePath': 'scenes/art-workflow.aescene',
            'sourceAsset': 'asset-id',
            'sceneRefs': {'SpriteRenderer.sprite': 'asset-id', 'AnimationPlayer.clip': 'clip-id'},
            'clipFrames': ['frame'] * 8,
            'state': {'sourceSHA256': sha, 'document': {'frames': [{} for _ in range(8)]}},
            'projectFiles': files,
        }
        root.joinpath('native-source.json').write_text(json.dumps(witness))
        return source, witness

    def summarize(self):
        shell = (ROOT / 'tools/stage125-check.sh').read_text()
        marker = '  python3 -B - "$evidence" <<\'PY\'\n'
        self.assertEqual(shell.count(marker), 1)
        script = shell.split(marker, 1)[1].split('\nPY\n', 1)[0]
        result = subprocess.run([sys.executable, '-B', '-', str(self.root)], input=script,
                                text=True, capture_output=True, check=False)
        summary = json.loads(self.root.joinpath('summary.json').read_text())
        return result, summary

    def test_missing_producer_does_not_pass(self):
        result, summary = self.summarize()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(summary['passed'])
        self.assertFalse(summary['fileInteropProducer']['passed'])
        self.assertIn('missing', summary['fileInteropProducer']['error'])

    def test_valid_producer_and_test_results_pass(self):
        self.producer()
        result, summary = self.summarize()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(summary['passed'])
        self.assertTrue(summary['fileInteropProducer']['passed'])

    def test_corrupt_source_and_incomplete_witness_fail(self):
        source, witness = self.producer()
        source.write_bytes(b'external modification')
        result, summary = self.summarize()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(summary['fileInteropProducer']['passed'])
        source.write_bytes(b'synthetic drawing')
        witness['state']['document']['frames'].pop()
        (self.root / 'interoperability/native-source.json').write_text(json.dumps(witness))
        result, summary = self.summarize()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(summary['fileInteropProducer']['passed'])

    def test_producer_does_not_replace_the_required_scenario(self):
        self.producer()
        event = {'Test': 'TestArtSceneAuthoringWorkflowNative', 'Action': 'skip'}
        self.root.joinpath('events.jsonl').write_text(json.dumps(event) + '\n')
        result, summary = self.summarize()
        self.assertNotEqual(result.returncode, 0)
        self.assertTrue(summary['fileInteropProducer']['passed'])
        self.assertFalse(summary['passed'])
        self.assertEqual(summary['required']['TestArtSceneAuthoringWorkflowNative'], 'SKIP')

    def test_missing_screenshot_and_modified_scene_fail(self):
        self.producer()
        screenshot = self.root / 'interoperability/native-source.png'
        screenshot.unlink()
        result, summary = self.summarize()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(summary['fileInteropProducer']['passed'])
        screenshot.write_bytes(b'\x89PNG\r\n\x1a\nsynthetic')
        (self.root / 'interoperability/project/scenes/art-workflow.aescene').write_bytes(b'changed')
        result, summary = self.summarize()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(summary['fileInteropProducer']['passed'])

    def test_project_path_traversal_is_rejected(self):
        _, witness = self.producer()
        witness['projectFiles']['../outside'] = 'not-a-hash'
        (self.root / 'interoperability/native-source.json').write_text(json.dumps(witness))
        result, summary = self.summarize()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('invalid', summary['fileInteropProducer']['error'])


if __name__ == '__main__':
    unittest.main()
