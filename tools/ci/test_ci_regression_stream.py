"""Exercise the actual Gitea regression step with disposable command stubs."""
import os
from pathlib import Path
import select
import shutil
import subprocess
import tempfile
import textwrap
import unittest


ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = ROOT / '.github/workflows/ci.yml'


def regression_step():
    text = WORKFLOW.read_text(encoding='utf-8')
    start = '      - name: Full Aego regression and component Try\n'
    if text.count(start) != 1:
        raise AssertionError('regression step is missing or ambiguous')
    rest = text.split(start, 1)[1]
    if '\n      - name: ' not in rest:
        raise AssertionError('regression step end is missing')
    section = rest.split('\n      - name: ', 1)[0]
    marker = '        run: |\n'
    if section.count(marker) != 1:
        raise AssertionError('regression shell block is missing or ambiguous')
    return textwrap.dedent(section.split(marker, 1)[1])


class RegressionStreamTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix='aego-ci-stream-test-')
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.repo = self.root / 'repo'
        (self.repo / 'tools/websmoke').mkdir(parents=True)
        self.runner = self.root / 'runner'
        artifact = self.runner / 'aego-artstudio-evidence'
        (artifact / 'artifact').mkdir(parents=True)
        (artifact / 'manifest.sha256').write_text('fake-manifest\n', encoding='utf-8')
        (artifact / 'core-deferred.txt').write_text('Deferred: required make ci must run studio-web-test.\n', encoding='utf-8')
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        self.marker = self.root / 'verified'
        self.make_call = self.root / 'make-call'
        self.firefox = self.root / 'browsers with spaces' / 'firefox'
        self.firefox.parent.mkdir()
        self.firefox.write_text('#!/bin/sh\nexit 0\n', encoding='utf-8')
        self.firefox.chmod(0o755)
        self._stub('npm', '#!/bin/sh\nexit 0\n')
        self._stub('node', '''#!/bin/sh
case "$*" in
  *chromium.executablePath*) printf '%s\\n' /fake/chromium ;;
  *firefox.executablePath*) printf '%s\\n' "$MOCK_FIREFOX_EXECUTABLE" ;;
  *) exit 9 ;;
esac
''')
        self._stub('xvfb-run', '#!/bin/sh\nshift 3\nexec "$@"\n')
        self._stub('make', '''#!/bin/sh
printf '%s\\n' "$*" >"$MOCK_MAKE_CALL"
printf '%s\\n' "$ARTSTUDIO_PYTHON" >>"$MOCK_MAKE_CALL"
printf '%s\\n' "${FIREFOX-<unset>}" >>"$MOCK_MAKE_CALL"
printf 'make stdout\\n'
printf 'make stderr\\n' >&2
if [ -n "${MOCK_RELEASE_FILE:-}" ]; then
    while [ ! -e "$MOCK_RELEASE_FILE" ]; do sleep 0.02; done
fi
exit "${MOCK_MAKE_EXIT:-0}"
''')
        self._stub('python3', '#!/bin/sh\nprintf verified >"$MOCK_VERIFY_MARKER"\n')
        tee = shutil.which('tee')
        if not tee:
            self.fail('tee is required for the CI logging test')
        self._stub('tee', f'''#!/bin/sh
if [ "${{MOCK_TEE_FAIL:-0}}" = 1 ]; then
    "{tee}" "$@" >/dev/null
    exit 23
fi
exec "{tee}" "$@"
''')
        self.env = os.environ.copy()
        self.env.update(PATH=str(self.bin) + os.pathsep + self.env['PATH'],
                        RUNNER_TEMP=str(self.runner), MOCK_VERIFY_MARKER=str(self.marker),
                        MOCK_MAKE_CALL=str(self.make_call), MOCK_FIREFOX_EXECUTABLE=str(self.firefox))
        for name in ('FIREFOX', 'MOCK_MAKE_EXIT', 'MOCK_TEE_FAIL', 'MOCK_RELEASE_FILE'):
            self.env.pop(name, None)

    def _stub(self, name, content):
        path = self.bin / name
        path.write_text(content, encoding='utf-8')
        path.chmod(0o755)

    def _run(self):
        return subprocess.run(['bash', '--noprofile', '--norc', '-e', '-o', 'pipefail',
                               '-c', regression_step()], cwd=self.repo, env=self.env,
                              stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                              text=True, timeout=10)

    def _log(self):
        return (self.runner / 'aego-full-ci/make-ci.log').read_text(encoding='utf-8')

    def _exit(self):
        return (self.runner / 'aego-full-ci/make-ci.exit').read_text(encoding='utf-8').strip()

    def test_success_preserves_output_and_runs_verifier(self):
        result = self._run()
        self.assertEqual(result.returncode, 0, result.stdout)
        self.assertEqual(self._exit(), '0')
        self.assertEqual(self._log(), 'make stdout\nmake stderr\n')
        self.assertIn(self._log(), result.stdout)
        self.assertTrue(self.marker.is_file())
        self.assertEqual(self.make_call.read_text().splitlines(), [
            'ci XVFB= CHROMIUM=/fake/chromium',
            str(self.runner / 'aego-artstudio-python/bin/python'),
            str(self.firefox),
        ])

    def test_invalid_firefox_never_runs_make(self):
        for executable in ('', str(self.root / 'missing'), str(self.root / 'not-executable'), str(self.root)):
            with self.subTest(executable=executable):
                (self.root / 'not-executable').write_text('not executable\n', encoding='utf-8')
                self.env['MOCK_FIREFOX_EXECUTABLE'] = executable
                result = self._run()
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertFalse(self.make_call.exists())
                self.assertFalse(self.marker.exists())

    def test_missing_or_wrong_deferred_marker_never_runs_make(self):
        marker = self.runner / 'aego-artstudio-evidence/core-deferred.txt'
        for value in (None, 'core passed', ''):
            with self.subTest(value=value):
                if value is None:
                    marker.unlink(missing_ok=True)
                else:
                    marker.write_text(value, encoding='utf-8')
                result = self._run()
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertFalse(self.make_call.exists())
                self.assertFalse(self.marker.exists())

    def test_make_failure_is_not_masked_by_tee(self):
        self.env['MOCK_MAKE_EXIT'] = '17'
        result = self._run()
        self.assertEqual(result.returncode, 17, result.stdout)
        self.assertEqual(self._exit(), '17')
        self.assertEqual(self._log(), 'make stdout\nmake stderr\n')
        self.assertFalse(self.marker.exists())

    def test_log_failure_fails_closed_even_if_make_succeeds(self):
        self.env['MOCK_TEE_FAIL'] = '1'
        result = self._run()
        self.assertEqual(result.returncode, 23, result.stdout)
        self.assertEqual(self._exit(), '0')
        self.assertIn('Failed to stream or save the full CI log', result.stdout)
        self.assertFalse(self.marker.exists())

    def test_output_is_visible_before_make_finishes(self):
        release = self.root / 'release'
        self.env['MOCK_RELEASE_FILE'] = str(release)
        process = subprocess.Popen(['bash', '--noprofile', '--norc', '-e', '-o', 'pipefail',
                                    '-c', regression_step()], cwd=self.repo, env=self.env,
                                   stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        try:
            readable, _, _ = select.select([process.stdout], [], [], 5)
            self.assertTrue(readable, 'make output was not streamed while the command was running')
            self.assertIn(b'make stdout', process.stdout.readline())
            self.assertIsNone(process.poll(), 'make should still be waiting')
            release.touch()
            _, _ = process.communicate(timeout=10)
            self.assertEqual(process.returncode, 0)
            self.assertTrue(self.marker.is_file())
        finally:
            if process.poll() is None:
                release.touch()
                process.kill()
                process.communicate(timeout=10)


if __name__ == '__main__':
    unittest.main()
