"""Check the sequential CI jobs and their pinned snapshot handoff."""
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import textwrap
import unittest


ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = ROOT / '.github/workflows/ci.yml'


def jobs():
    text = WORKFLOW.read_text(encoding='utf-8').split('\njobs:\n', 1)[1]
    matches = list(re.finditer(r'^  ([a-z][a-z0-9-]*):\s*$', text, re.MULTILINE))
    return {match.group(1): text[match.end():matches[i + 1].start() if i + 1 < len(matches) else len(text)]
            for i, match in enumerate(matches)}


def step_shell(job, name):
    marker = f'      - name: {name}\n'
    if job.count(marker) != 1:
        raise AssertionError(f'step {name!r} is missing or ambiguous')
    section = job.split(marker, 1)[1].split('\n      - ', 1)[0]
    marker = '        run: |\n'
    if section.count(marker) != 1:
        raise AssertionError(f'shell block for {name!r} is missing or ambiguous')
    return textwrap.dedent(section.split(marker, 1)[1])


class SnapshotHandoffTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix='aego-ci-handoff-test-')
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.repo = self.root / 'repo'
        self.repo.mkdir()
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        self.runner = self.root / 'runner'
        self.snapshot = self.runner / 'aego-artstudio-evidence'
        self.artifact = self.snapshot / 'artifact'
        self.artifact.mkdir(parents=True)
        self.sha = 'a' * 40
        self.manifest = self.artifact / 'manifest.json'
        self._manifest(self.sha, False)
        self.pin = hashlib.sha256(self.manifest.read_bytes()).hexdigest()
        (self.snapshot / 'manifest.sha256').write_text(self.pin + '\n', encoding='ascii')
        (self.snapshot / 'head.txt').write_text(self.sha + '\n', encoding='ascii')
        self.calls = self.root / 'go-calls'
        self._stub('git', '#!/bin/sh\nprintf "%s\\n" "$MOCK_GIT_HEAD"\n')
        self._stub('go', '''#!/bin/sh
printf '%s\\n' "$*" >> "$MOCK_GO_CALLS"
case "$3" in
  verify)
    [ "$4" = -dir ] && [ "$6" = -sha256 ] || exit 22
    actual=$(sha256sum "$5/manifest.json") || exit 22
    [ "${actual%% *}" = "$7" ] || exit 21
    ;;
  check-source)
    [ "${MOCK_SOURCE_FAIL:-0}" = 0 ] || exit 24
    ;;
  *) exit 25 ;;
esac
''')
        self.env = os.environ.copy()
        self.env.update(PATH=str(self.bin) + os.pathsep + self.env['PATH'],
                        RUNNER_TEMP=str(self.runner), GITHUB_SHA=self.sha, MOCK_GIT_HEAD=self.sha,
                        EXPECTED_STUDIO_MANIFEST=self.pin, MOCK_GO_CALLS=str(self.calls))
        self.env.pop('MOCK_SOURCE_FAIL', None)

    def _stub(self, name, content):
        path = self.bin / name
        path.write_text(content, encoding='utf-8')
        path.chmod(0o755)

    def _manifest(self, commit, release):
        self.manifest.write_text(json.dumps({'commit': commit, 'release': release}) + '\n', encoding='utf-8')

    def _run(self, job):
        if self.calls.exists():
            self.calls.unlink()
        shell = step_shell(jobs()[job], 'Verify the tested Studio snapshot')
        return subprocess.run(['bash', '--noprofile', '--norc', '-e', '-o', 'pipefail', '-c', shell],
                              cwd=self.repo, env=self.env, stdout=subprocess.PIPE,
                              stderr=subprocess.STDOUT, text=True, timeout=10)

    def test_job_chain_retains_all_required_gates(self):
        all_jobs = jobs()
        self.assertEqual(list(all_jobs), ['studio-snapshot', 'checks', 'native-authoring', 'build', 'release'])
        self.assertIn('needs: studio-snapshot', all_jobs['checks'])
        self.assertIn('needs: [studio-snapshot, checks]', all_jobs['native-authoring'])
        self.assertIn('needs: [checks, native-authoring]', all_jobs['build'])
        self.assertIn('always() && github.server_url', all_jobs['build'])
        self.assertIn('needs: [studio-snapshot, checks, native-authoring, build]', all_jobs['release'])
        self.assertIn('bash tools/ci/artstudio_snapshot_e2e.sh', all_jobs['studio-snapshot'])
        self.assertIn('steps.snapshot_pin.outputs.sha256', all_jobs['studio-snapshot'])
        self.assertIn('make ci XVFB=', all_jobs['checks'])
        self.assertIn('component_try_evidence.py verify', all_jobs['checks'])
        for gate in ('bash tools/stage125-check.sh artstudio-native', 'file_interop_e2e.py',
                     'make remote-examples-check', 'make nsis-check'):
            self.assertIn(gate, all_jobs['native-authoring'])
        for job in ('checks', 'native-authoring'):
            self.assertIn('needs.studio-snapshot.outputs.manifest_sha256', all_jobs[job])
            self.assertIn('asset-studio-snapshot-${{ github.run_id }}-${{ github.run_attempt }}', all_jobs[job])
            self.assertIn('gitea-download-artifact@75635f32b4c1c41c4b3d64e8f85210112ed4c9c7', all_jobs[job])

    def test_producer_pins_only_a_valid_hash_for_the_exact_head(self):
        output = self.root / 'github-output'
        self.env['GITHUB_OUTPUT'] = str(output)
        shell = step_shell(jobs()['studio-snapshot'], 'Pin the tested Studio snapshot')
        def run():
            return subprocess.run(['bash', '--noprofile', '--norc', '-e', '-o', 'pipefail', '-c', shell],
                                  cwd=self.repo, env=self.env, stdout=subprocess.PIPE,
                                  stderr=subprocess.STDOUT, text=True, timeout=10)
        result = run()
        self.assertEqual(result.returncode, 0, result.stdout)
        self.assertEqual(output.read_text(), 'sha256=' + self.pin + '\n')
        output.unlink()
        (self.snapshot / 'head.txt').write_text('b' * 40 + '\n')
        self.assertNotEqual(run().returncode, 0)
        self.assertFalse(output.exists())
        (self.snapshot / 'head.txt').write_text(self.sha + '\n')
        (self.snapshot / 'manifest.sha256').write_text('not-a-sha\n')
        self.assertNotEqual(run().returncode, 0)
        self.assertFalse(output.exists())

    def test_valid_snapshot_checks_bytes_and_source_in_both_jobs(self):
        for job in ('checks', 'native-authoring'):
            with self.subTest(job=job):
                result = self._run(job)
                self.assertEqual(result.returncode, 0, result.stdout)
                calls = self.calls.read_text(encoding='utf-8').splitlines()
                self.assertEqual(len(calls), 2)
                self.assertIn(' verify ', ' ' + calls[0] + ' ')
                self.assertIn(' check-source ', ' ' + calls[1] + ' ')
                self.assertIn(self.pin, calls[0])

    def test_missing_or_mismatched_pin_and_head_are_rejected(self):
        scenarios = ('bad-pin', 'pin-file', 'snapshot-head', 'checkout-head')
        for job in ('checks', 'native-authoring'):
            for scenario in scenarios:
                with self.subTest(job=job, scenario=scenario):
                    self.env['EXPECTED_STUDIO_MANIFEST'] = 'bad' if scenario == 'bad-pin' else self.pin
                    (self.snapshot / 'manifest.sha256').write_text(('0' * 64 if scenario == 'pin-file' else self.pin) + '\n')
                    (self.snapshot / 'head.txt').write_text(('b' * 40 if scenario == 'snapshot-head' else self.sha) + '\n')
                    self.env['MOCK_GIT_HEAD'] = 'b' * 40 if scenario == 'checkout-head' else self.sha
                    result = self._run(job)
                    self.assertNotEqual(result.returncode, 0, result.stdout)
                    self.assertFalse(self.calls.exists())

    def test_wrong_manifest_identity_fails_even_with_python_optimization(self):
        self.env['PYTHONOPTIMIZE'] = '1'
        for job in ('checks', 'native-authoring'):
            for commit, release in (('b' * 40, False), (self.sha, True), (self.sha, None)):
                with self.subTest(job=job, commit=commit, release=release):
                    self._manifest(commit, release)
                    result = self._run(job)
                    self.assertNotEqual(result.returncode, 0, result.stdout)
                    self.assertIn('snapshot identity mismatch', result.stdout)
                    self.assertFalse(self.calls.exists())

    def test_changed_manifest_bytes_and_source_mismatch_fail_closed(self):
        for job in ('checks', 'native-authoring'):
            with self.subTest(job=job, scenario='manifest bytes'):
                self.manifest.write_text(self.manifest.read_text() + ' ')
                result = self._run(job)
                self.assertEqual(result.returncode, 21, result.stdout)
                self.assertEqual(len(self.calls.read_text().splitlines()), 1)
                self._manifest(self.sha, False)
            with self.subTest(job=job, scenario='source'):
                self.env['MOCK_SOURCE_FAIL'] = '1'
                result = self._run(job)
                self.assertEqual(result.returncode, 24, result.stdout)
                self.assertEqual(len(self.calls.read_text().splitlines()), 2)
                self.env.pop('MOCK_SOURCE_FAIL')


if __name__ == '__main__':
    unittest.main()
