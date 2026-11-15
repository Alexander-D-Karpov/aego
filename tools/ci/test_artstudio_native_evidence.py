import hashlib
import json
import os
import shutil
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


class FileInteropSourceIdentityTests(unittest.TestCase):
    """Exercise the real runner up to source verification in a private repo."""

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='aego-interop-identity-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.repo = self.root / 'repo'
        self.repo.mkdir()
        self.env = os.environ.copy()
        for name in ('PYTHONDONTWRITEBYTECODE', 'PYTHONPYCACHEPREFIX', 'PYTHONOPTIMIZE',
                     'GIT_DIR', 'GIT_WORK_TREE', 'GIT_INDEX_FILE', 'GIT_COMMON_DIR',
                     'GIT_OBJECT_DIRECTORY', 'GIT_ALTERNATE_OBJECT_DIRECTORIES', 'GIT_CONFIG_COUNT'):
            self.env.pop(name, None)
        self.env.update(GIT_CONFIG_GLOBAL=os.devnull, GIT_CONFIG_NOSYSTEM='1')
        for name in (
            'tools/artstudio/release/tests/file_interop_e2e.py',
            'tools/artstudio/release/tests/browser_context.py',
            'tools/ci/component_try_evidence.py',
            'tools/ci/worktree_manifest.py',
        ):
            target = self.repo / name
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(ROOT / name, target)

        def git(*args):
            subprocess.run(['git', '-C', str(self.repo), *args], check=True,
                           env=self.env, stdout=subprocess.PIPE, stderr=subprocess.PIPE)

        template = self.root / 'empty-git-template'
        template.mkdir()
        git('init', '-q', '--template', str(template))
        git('add', '.')
        git('-c', 'user.name=Aego test', '-c', 'user.email=test@example.invalid',
            '-c', 'commit.gpgsign=false', 'commit', '-q', '-m', 'fixture')
        snapshot = self.root / 'native-source-before.json'
        subprocess.run([sys.executable, '-B', str(self.repo / 'tools/ci/component_try_evidence.py'),
                        'snapshot', '--root', str(self.repo), '--output', str(snapshot)],
                       env=self.env, check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self.before = json.loads(snapshot.read_text())
        self.native = self.root / 'native'
        self.native.mkdir()
        (self.native / 'summary.json').write_text(json.dumps({
            'passed': True,
            'required': {'TestArtSceneAuthoringWorkflowNative': 'PASS'},
            'sourceFingerprint': self.before['worktreeSHA256'],
        }))
        self.artifact = self.root / 'artifact'
        self.artifact.mkdir()
        (self.artifact / 'manifest.json').write_text(json.dumps({
            'buildID': 'synthetic', 'sourceSHA256': 'synthetic',
        }))
        binary = self.root / 'bin'
        binary.mkdir()
        go = binary / 'go'
        go.write_text('#!/bin/sh\nprintf "%s\\n" "$*" >> "$MOCK_GO_TRACE"\nexit 73\n')
        go.chmod(0o755)
        self.trace = self.root / 'go-trace'
        self.env['PATH'] = str(binary) + os.pathsep + self.env['PATH']
        self.env['MOCK_GO_TRACE'] = str(self.trace)

    def run_interop(self):
        out = self.root / 'interop'
        command = [sys.executable, str(self.repo / 'tools/artstudio/release/tests/file_interop_e2e.py'),
                   '--artifact', str(self.artifact), '--native-evidence', str(self.native),
                   '--evidence', str(out), '--app-url', 'http://127.0.0.1:8097/']
        result = subprocess.run(command, cwd=self.repo, env=self.env, capture_output=True,
                                text=True, timeout=30)
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        return json.loads((out / 'result.json').read_text()), json.loads((out / 'source-before.json').read_text())

    def test_runner_does_not_invalidate_native_source_with_bytecode(self):
        report, before = self.run_interop()
        self.assertEqual(before, self.before)
        self.assertEqual(list(self.repo.rglob('*.pyc')), [])
        self.assertIn('Command exited 73', report['error'])
        self.assertIn('check-source', self.trace.read_text())

    def test_runner_still_rejects_a_changed_tracked_source(self):
        path = self.repo / 'tools/artstudio/release/tests/browser_context.py'
        path.write_text(path.read_text() + '\n# external source change\n')
        report, before = self.run_interop()
        self.assertNotEqual(before['worktreeSHA256'], self.before['worktreeSHA256'])
        self.assertIn('Native producer was tested on another source tree', report['error'])
        self.assertFalse(self.trace.exists())

    def test_runner_still_rejects_a_preexisting_untracked_pyc(self):
        path = self.repo / 'tools/ci/__pycache__/unowned.pyc'
        path.parent.mkdir()
        path.write_bytes(b'unowned bytecode sentinel')
        report, before = self.run_interop()
        self.assertNotEqual(before['worktreeSHA256'], self.before['worktreeSHA256'])
        self.assertIn('Native producer was tested on another source tree', report['error'])
        self.assertFalse(self.trace.exists())
        self.assertEqual(path.read_bytes(), b'unowned bytecode sentinel')


if __name__ == '__main__':
    unittest.main()
