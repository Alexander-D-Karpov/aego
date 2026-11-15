"""Check the combined CI job and its pinned Studio snapshot."""
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import textwrap
import unittest


ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = ROOT / '.github/workflows/ci.yml'
VERIFY_STEPS = ('Verify Studio snapshot before regression', 'Verify Studio snapshot before native authoring')


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

    def _run(self, step):
        if self.calls.exists():
            self.calls.unlink()
        job = 'checks' if step == VERIFY_STEPS[0] else 'native-authoring'
        shell = step_shell(jobs()[job], step)
        return subprocess.run(['bash', '--noprofile', '--norc', '-e', '-o', 'pipefail', '-c', shell],
                              cwd=self.repo, env=self.env, stdout=subprocess.PIPE,
                              stderr=subprocess.STDOUT, text=True, timeout=10)

    def test_job_chain_retains_all_required_gates(self):
        all_jobs = jobs()
        self.assertEqual(list(all_jobs), ['checks', 'native-authoring', 'build', 'release'])
        checks = all_jobs['checks']
        native = all_jobs['native-authoring']
        self.assertIn('timeout-minutes: 165', checks)
        self.assertIn('needs: checks', native)
        self.assertIn('needs: [checks, native-authoring]', all_jobs['build'])
        self.assertIn('always() && github.server_url', all_jobs['build'])
        self.assertIn('needs: [checks, native-authoring, build]', all_jobs['release'])
        self.assertIn('if: ${{ false }}', all_jobs['release'])
        self.assertIn('bash tools/ci/artstudio_snapshot_e2e.sh', checks)
        self.assertIn('ARTSTUDIO_SNAPSHOT_CORE: defer-to-make-ci', checks)
        self.assertEqual(checks.count('steps.snapshot_pin.outputs.sha256'), 2)
        self.assertIn('needs.checks.outputs.manifest_sha256', native)
        self.assertIn('actions/checkout@v4', native)
        self.assertIn('gitea-download-artifact@75635f32b4c1c41c4b3d64e8f85210112ed4c9c7', native)
        self.assertIn('asset-studio-snapshot-${{ github.run_id }}-${{ github.run_attempt }}', native)
        self.assertIn('make ci XVFB=', checks)
        self.assertIn('component_try_evidence.py verify', checks)
        for gate in ('bash tools/stage125-check.sh artstudio-native', 'file_interop_e2e.py',
                     'make remote-examples-check', 'make nsis-check'):
            self.assertIn(gate, native)
        steps = ('Asset Studio snapshot and real browser integration', 'Pin the tested Studio snapshot',
                 VERIFY_STEPS[0], 'Full Aego regression and component Try')
        positions = [checks.index('- name: ' + step) for step in steps]
        self.assertEqual(positions, sorted(positions))
        native_steps = ('Download the tested Studio snapshot', VERIFY_STEPS[1],
                        'Native authoring and installed Linux protocol', 'installer dry run')
        positions = [native.index('- name: ' + step) for step in native_steps]
        self.assertEqual(positions, sorted(positions))
        for name in ('Preserve Asset Studio snapshot evidence', 'Preserve full CI and component evidence'):
            section = checks.split('- name: ' + name + '\n', 1)[1].split('\n      - ', 1)[0]
            self.assertIn('if: ${{ always() }}', section)
            self.assertIn('gitea-upload-artifact@81f940d004763f986ba3582c007fd842dd5cb0d7', section)
        native_evidence = native.split('- name: Preserve native authoring evidence\n', 1)[1].split('\n      - ', 1)[0]
        self.assertIn('if: ${{ always() }}', native_evidence)
        self.assertIn('gitea-upload-artifact@81f940d004763f986ba3582c007fd842dd5cb0d7', native_evidence)
        self.assertNotIn('gitea-download-artifact@', checks)
        makefile = (ROOT / 'Makefile').read_text(encoding='utf-8')
        self.assertRegex(makefile, r'(?m)^ci: studio-web-test\s*$')
        self.assertIn("VET_PKGS = $$(go list ./... | grep -v -e '/gpu/gl33/gl$$')", makefile)
        self.assertIn('go vet -cgocall=false -unsafeptr=false $(VET_PKGS)', makefile)
        self.assertIn('go vet ./platform/sdl', makefile)
        self.assertIn(' lint depcheck ', makefile)
        for gate in ('go build ./...', 'go test ./...', 'aego_nonative', 'aego_gltest',
                     'aego_golden', '--demo headless', 'example project'):
            self.assertIn(gate, all_jobs['build'])

    def test_producer_pins_only_a_valid_hash_for_the_exact_head(self):
        output = self.root / 'github-output'
        self.env['GITHUB_OUTPUT'] = str(output)
        shell = step_shell(jobs()['checks'], 'Pin the tested Studio snapshot')
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

    def test_valid_snapshot_checks_bytes_and_source_before_both_gates(self):
        for step in VERIFY_STEPS:
            with self.subTest(step=step):
                result = self._run(step)
                self.assertEqual(result.returncode, 0, result.stdout)
                calls = self.calls.read_text(encoding='utf-8').splitlines()
                self.assertEqual(len(calls), 2)
                self.assertIn(' verify ', ' ' + calls[0] + ' ')
                self.assertIn(' check-source ', ' ' + calls[1] + ' ')
                self.assertIn(self.pin, calls[0])

    def test_missing_or_mismatched_pin_and_head_are_rejected(self):
        scenarios = ('bad-pin', 'pin-file', 'snapshot-head', 'checkout-head')
        for step in VERIFY_STEPS:
            for scenario in scenarios:
                with self.subTest(step=step, scenario=scenario):
                    self.env['EXPECTED_STUDIO_MANIFEST'] = 'bad' if scenario == 'bad-pin' else self.pin
                    (self.snapshot / 'manifest.sha256').write_text(('0' * 64 if scenario == 'pin-file' else self.pin) + '\n')
                    (self.snapshot / 'head.txt').write_text(('b' * 40 if scenario == 'snapshot-head' else self.sha) + '\n')
                    self.env['MOCK_GIT_HEAD'] = 'b' * 40 if scenario == 'checkout-head' else self.sha
                    result = self._run(step)
                    self.assertNotEqual(result.returncode, 0, result.stdout)
                    self.assertFalse(self.calls.exists())

    def test_wrong_manifest_identity_fails_even_with_python_optimization(self):
        self.env['PYTHONOPTIMIZE'] = '1'
        for step in VERIFY_STEPS:
            for commit, release in (('b' * 40, False), (self.sha, True), (self.sha, None)):
                with self.subTest(step=step, commit=commit, release=release):
                    self._manifest(commit, release)
                    result = self._run(step)
                    self.assertNotEqual(result.returncode, 0, result.stdout)
                    self.assertIn('snapshot identity mismatch', result.stdout)
                    self.assertFalse(self.calls.exists())

    def test_changed_manifest_bytes_and_source_mismatch_fail_closed(self):
        for step in VERIFY_STEPS:
            with self.subTest(step=step, scenario='manifest bytes'):
                self.manifest.write_text(self.manifest.read_text() + ' ')
                result = self._run(step)
                self.assertEqual(result.returncode, 21, result.stdout)
                self.assertEqual(len(self.calls.read_text().splitlines()), 1)
                self._manifest(self.sha, False)
            with self.subTest(step=step, scenario='source'):
                self.env['MOCK_SOURCE_FAIL'] = '1'
                result = self._run(step)
                self.assertEqual(result.returncode, 24, result.stdout)
                self.assertEqual(len(self.calls.read_text().splitlines()), 2)
                self.env.pop('MOCK_SOURCE_FAIL')


class SnapshotCoreModeTest(unittest.TestCase):
    """Stop the real snapshot script at stubbed commands, before any build."""

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix='aego-ci-snapshot-mode-')
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.repo = self.root / 'repo'
        runner = self.repo / 'tools/ci/artstudio_snapshot_e2e.sh'
        runner.parent.mkdir(parents=True)
        shutil.copyfile(ROOT / 'tools/ci/artstudio_snapshot_e2e.sh', runner)
        self.runner = runner
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        self.marker = self.root / 'calls'
        self.snapshot = self.root / 'snapshot'
        self._stub('git', '''#!/bin/sh
case "$1" in rev-parse) printf 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\\n';; status) :;; *) exit 9;; esac
''')
        self._stub('go', '''#!/bin/sh
if [ "$1" = version ]; then printf 'go fixture\\n'; exit 0; fi
printf 'go %s\\n' "$*" >>"$MOCK_CALLS"
exit 29
''')
        self._stub('make', '''#!/bin/sh
printf 'make %s\\n' "$*" >>"$MOCK_CALLS"
exit 41
''')
        python = self.bin / 'python-fixture'
        python.write_text(f'''#!{sys.executable}
import os, sys
from pathlib import Path
if sys.argv[1:] == ['-c', 'import playwright.sync_api']:
    raise SystemExit(0)
if sys.argv[1:3] == ['-m', 'playwright']:
    print('playwright fixture')
    raise SystemExit(0)
if sys.argv[1] == '-':
    os.execv(sys.executable, [sys.executable, '-B'] + sys.argv[1:])
if sys.argv[1] == 'tools/ci/worktree_manifest.py':
    Path(sys.argv[sys.argv.index('--output') + 1]).write_text('{{}}')
    raise SystemExit(0)
raise SystemExit(91)
''', encoding='utf-8')
        python.chmod(0o755)
        self.env = dict(os.environ, PATH=str(self.bin) + os.pathsep + os.environ['PATH'],
                        ARTSTUDIO_PYTHON=str(python), ARTSTUDIO_SNAPSHOT_DIR=str(self.snapshot),
                        MOCK_CALLS=str(self.marker))
        self.env.pop('ARTSTUDIO_SNAPSHOT_CORE', None)

    def _stub(self, name, contents):
        path = self.bin / name
        path.write_text(contents, encoding='utf-8')
        path.chmod(0o755)

    def _run(self):
        return subprocess.run(['bash', str(self.runner)], env=self.env, text=True,
                              stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=10)

    def test_standalone_snapshot_requires_core_by_default(self):
        result = self._run()
        self.assertEqual(result.returncode, 41, result.stdout)
        self.assertEqual(self.marker.read_text().splitlines(), ['make studio-web-test'])
        self.assertEqual((self.snapshot / 'core.exit').read_text().strip(), '41')
        self.assertFalse((self.snapshot / 'core-deferred.txt').exists())

    def test_combined_ci_explicitly_defers_core_without_a_fake_pass(self):
        self.env['ARTSTUDIO_SNAPSHOT_CORE'] = 'defer-to-make-ci'
        result = self._run()
        self.assertEqual(result.returncode, 29, result.stdout)
        self.assertEqual(self.marker.read_text().splitlines(), [
            f'go build -o {self.snapshot}/artstudio-release ./tools/artstudio/release'])
        self.assertEqual((self.snapshot / 'core-deferred.txt').read_text().strip(),
                         'Deferred: required make ci must run studio-web-test.')
        self.assertFalse((self.snapshot / 'core.exit').exists())

    def test_unknown_mode_fails_before_creating_evidence(self):
        self.env['ARTSTUDIO_SNAPSHOT_CORE'] = 'skip'
        result = self._run()
        self.assertEqual(result.returncode, 2, result.stdout)
        self.assertFalse(self.snapshot.exists())
        self.assertFalse(self.marker.exists())


if __name__ == '__main__':
    unittest.main()
