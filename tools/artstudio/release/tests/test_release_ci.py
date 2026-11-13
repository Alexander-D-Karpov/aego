"""Exercise interpreter selection and failure handling without running a gate.

All expensive commands are stubs, and the synthetic run intentionally stops
before a gate report could be written. This is not release evidence.
"""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[4]
SCRIPT = ROOT / 'tools/artstudio/release/ci.sh'


class ReleaseCIRunnerTest(unittest.TestCase):
    def run_fixture(self, *, configured, custom_parity=False, fail='', missing=False):
        with tempfile.TemporaryDirectory(prefix='aego-release-ci-test-') as directory:
            root = Path(directory)
            bin_dir = root / 'bin'
            bin_dir.mkdir()
            trace = root / 'trace'
            artifact = root / 'artifact'
            (artifact / 'assets').mkdir(parents=True)
            (artifact / 'assets/core.wasm').write_bytes(b'synthetic')
            (artifact / 'manifest.json').write_text(json.dumps({'wasm': {'path': 'assets/core.wasm'}}))
            evidence = root / 'evidence'

            def executable(path, body):
                path.write_text('#!/bin/sh\nset -eu\n' + body)
                path.chmod(0o700)

            def python_stub(path, label):
                executable(path, f'''printf '%s|%s\\n' '{label}' "$*" >> "$AEGO_CI_TEST_TRACE"
for arg in "$@"; do
  if [ "$arg" = '-' ]; then exec "$AEGO_CI_TEST_REAL_PYTHON" "$@"; fi
done
case "$*" in
  *"$AEGO_CI_TEST_FAIL"*) if [ -n "$AEGO_CI_TEST_FAIL" ]; then exit 23; fi ;;
esac
printf 'synthetic interpreter stub\\n'
''')

            python_stub(bin_dir / 'python3', 'default')
            selected = root / 'python with spaces'
            python_stub(selected, 'selected')
            if missing:
                selected.unlink()
            executable(bin_dir / 'bash', '''if [ "$1" = '-c' ]; then exec /bin/bash "$@"; fi
printf 'stub-bash|%s\\n' "$*" >> "$AEGO_CI_TEST_TRACE"
printf 'synthetic stage stub\\n'
''')
            for command in ('go', 'node'):
                executable(bin_dir / command, f'''printf '{command}|%s\\n' "$*" >> "$AEGO_CI_TEST_TRACE"
printf 'synthetic {command} stub\\n'
''')
            executable(bin_dir / 'make', '''printf 'make|%s\\n' "$*" >> "$AEGO_CI_TEST_TRACE"
printf 'synthetic make stub\\n'
if [ "$1" = 'ci' ]; then exit 37; fi
''')
            external = root / 'external parity runner'
            executable(external, '''printf 'custom-parity|%s\\n' "$*" >> "$AEGO_CI_TEST_TRACE"
printf 'synthetic custom parity stub\\n'
''')
            env = {k: v for k, v in os.environ.items()
                   if not k.startswith('ARTSTUDIO_') and k not in (
                       'AEGO_NATIVE_SCENARIO_RUNNER', 'AEGO_NATIVE_SCENARIO_EVIDENCE',
                       'AEGO_COMPONENT_TRY_RUNNER', 'AEGO_DOCS_INTEGRATION_RUNNER',
                       'AEGO_WASM_PARITY_RUNNER')}
            env.update(PATH=str(bin_dir) + os.pathsep + os.defpath,
                       ARTSTUDIO_ARTIFACT=str(artifact), ARTSTUDIO_EVIDENCE=str(evidence),
                       ARTSTUDIO_APP_URL='https://studio.invalid/',
                       ARTSTUDIO_DOCS_URL='https://docs.invalid/',
                       AEGO_CI_TEST_TRACE=str(trace), AEGO_CI_TEST_REAL_PYTHON=sys.executable,
                       AEGO_CI_TEST_FAIL=fail)
            if configured:
                env['ARTSTUDIO_PYTHON'] = str(selected)
            if custom_parity:
                env['AEGO_WASM_PARITY_RUNNER'] = str(external)
            result = subprocess.run(['/bin/bash', str(SCRIPT)], cwd=ROOT, env=env,
                                    text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                                    stdin=subprocess.DEVNULL, timeout=30)
            calls = trace.read_text().splitlines() if trace.exists() else []
            exits = {p.stem: p.read_text().strip() for p in evidence.glob('*.exit')}
            self.assertFalse((evidence / 'gate.json').exists(), 'a synthetic test must never write a gate report')
            return result, calls, exits

    def test_configured_python_runs_all_builtin_python_checks(self):
        result, calls, exits = self.run_fixture(configured=True)
        self.assertEqual(result.returncode, 1, result.stdout)
        self.assertEqual(exits.get('regression'), '37')
        python_calls = [call for call in calls if call.startswith(('default|', 'selected|'))]
        self.assertTrue(python_calls, calls)
        self.assertTrue(all(call.startswith('selected|') for call in python_calls), python_calls)
        for name in ('test_browser_context.py', 'file_interop_e2e.py', 'parity_e2e.py',
                     'browser_e2e.py', 'workspace_files_browser.py', 'pending_fields_browser.py', 'budgets.py'):
            self.assertTrue(any(name in call for call in python_calls), (name, python_calls))

    def test_default_and_explicit_external_parity_runner(self):
        for configured in (False, True):
            with self.subTest(configured=configured):
                result, calls, exits = self.run_fixture(configured=configured, custom_parity=True)
                self.assertEqual(result.returncode, 1, result.stdout)
                self.assertEqual(exits.get('regression'), '37')
                label = 'selected|' if configured else 'default|'
                self.assertTrue(any(call.startswith(label) and 'file_interop_e2e.py' in call for call in calls), calls)
                self.assertEqual(sum(call.startswith('custom-parity|') for call in calls), 1, calls)
                self.assertFalse(any('parity_e2e.py' in call for call in calls), calls)

    def test_configured_python_failure_stops_the_gate(self):
        result, calls, exits = self.run_fixture(configured=True, fail='browser_e2e.py')
        self.assertEqual(result.returncode, 1, result.stdout)
        self.assertEqual(exits.get('chromium'), '23')
        self.assertNotIn('regression', exits)
        self.assertTrue(any(call.startswith('selected|') and 'browser_e2e.py' in call for call in calls), calls)
        self.assertFalse(any(call == 'make|ci' for call in calls), calls)

    def test_missing_configured_python_never_falls_back(self):
        result, calls, exits = self.run_fixture(configured=True, missing=True)
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertFalse(any(call.startswith('default|') for call in calls), calls)
        self.assertEqual(exits, {})


if __name__ == '__main__':
    unittest.main()
