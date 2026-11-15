"""Exercise the real native-authoring workflow shell block with command stubs."""

import json
import os
from pathlib import Path
import select
import subprocess
import tempfile
import textwrap
import time
import unittest


ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = ROOT / '.github/workflows/ci.yml'


def native_step():
    text = WORKFLOW.read_text(encoding='utf-8')
    marker = '      - name: Native authoring and installed Linux protocol\n'
    if text.count(marker) != 1:
        raise ValueError('native authoring step is missing or ambiguous')
    section = text.split(marker, 1)[1].split('\n      - name: ', 1)[0]
    shell = '        run: |\n'
    if section.count(shell) != 1:
        raise ValueError('native authoring shell block is missing or ambiguous')
    return textwrap.dedent(section.split(shell, 1)[1])


class NativeStreamTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='aego-native-ci-stream-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.runner = self.root / 'runner'
        self.runner.mkdir()
        self.repo = self.root / 'repo'
        (self.repo / 'tools').mkdir(parents=True)
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        self.trace = self.root / 'trace'
        self.stub(self.repo / 'tools/acceptance-check.sh', '''#!/bin/sh
printf 'native|%s\\n' "$*" >> "$TRACE"
printf 'native output before completion\\n'
if [ "${WAIT_NATIVE:-0}" = 1 ]; then IFS= read -r release; fi
exit "${NATIVE_EXIT:-0}"
''')
        self.stub(self.runner / 'aego-artstudio-python/bin/python', '''#!/bin/sh
printf 'interop|%s\\n' "$*" >> "$TRACE"
printf 'interop fixture output\\n'
exit 0
''')
        self.stub(self.bin / 'make', '''#!/bin/sh
printf 'make|%s\\n' "$*" >> "$TRACE"
printf 'remote fixture output\\n'
exit 0
''')
        self.env = dict(os.environ, PATH=str(self.bin) + os.pathsep + os.defpath,
                        RUNNER_TEMP=str(self.runner), TRACE=str(self.trace), GITHUB_SHA='a' * 40)
        for name in ('WAIT_NATIVE', 'NATIVE_EXIT'):
            self.env.pop(name, None)

    def stub(self, path, text):
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text)
        path.chmod(0o755)

    def run_step(self):
        return subprocess.run(['bash', '-c', native_step()], cwd=self.repo, env=self.env,
                              text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=15)

    def evidence(self, name, suffix):
        return (self.runner / 'aego-full-ci' / (name + suffix)).read_text().strip()

    def test_success_runs_and_records_each_gate(self):
        result = self.run_step()
        self.assertEqual(result.returncode, 0, result.stdout)
        self.assertEqual([x.split('|', 1)[0] for x in self.trace.read_text().splitlines()],
                         ['native', 'interop', 'make'])
        for name in ('native-user-scenario', 'file-interoperability', 'remote-examples'):
            with self.subTest(name=name):
                self.assertEqual(self.evidence(name, '.exit'), '0')
                timing = json.loads(self.evidence(name, '.timing.json'))
                self.assertEqual(timing['suite_id'], name)
                self.assertEqual(timing['source_sha'], 'a' * 40)
                self.assertEqual(timing['exit_code'], 0)
                self.assertIsNone(timing['environment_digest'])
                self.assertEqual(timing['cache_status'], 'unmeasured')

    def test_command_failure_stops_later_gates(self):
        self.env['NATIVE_EXIT'] = '17'
        result = self.run_step()
        self.assertEqual(result.returncode, 17, result.stdout)
        self.assertEqual(self.evidence('native-user-scenario', '.exit'), '17')
        self.assertEqual(self.evidence('native-user-scenario', '.command.exit'), '17')
        self.assertEqual(self.evidence('native-user-scenario', '.stream.exit'), '0')
        self.assertEqual(len(self.trace.read_text().splitlines()), 1)

    def test_tee_failure_cannot_turn_into_success(self):
        self.stub(self.bin / 'tee', '#!/bin/sh\ncat >"$1"\nexit 73\n')
        result = self.run_step()
        self.assertEqual(result.returncode, 73, result.stdout)
        self.assertEqual(self.evidence('native-user-scenario', '.exit'), '73')
        self.assertEqual(self.evidence('native-user-scenario', '.command.exit'), '0')
        self.assertEqual(self.evidence('native-user-scenario', '.stream.exit'), '73')
        self.assertEqual(len(self.trace.read_text().splitlines()), 1)

    def test_output_is_available_before_native_gate_finishes(self):
        self.env['WAIT_NATIVE'] = '1'
        process = subprocess.Popen(['bash', '-c', native_step()], cwd=self.repo, env=self.env,
                                   stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        output = b''
        try:
            deadline = time.monotonic() + 8
            while b'native output before completion' not in output:
                remaining = deadline - time.monotonic()
                if remaining <= 0 or not select.select([process.stdout], [], [], remaining)[0]:
                    break
                chunk = os.read(process.stdout.fileno(), 4096)
                if not chunk:
                    break
                output += chunk
            self.assertIn(b'native output before completion', output)
            self.assertIsNone(process.poll())
            self.assertEqual(self.evidence('native-user-scenario', '.exit'), 'RUNNING')
            self.assertIn('native output before completion', self.evidence('native-user-scenario', '.log'))
        finally:
            rest, _ = process.communicate(input=b'continue\n', timeout=15)
            output += rest
        self.assertEqual(process.returncode, 0, output.decode(errors='replace'))

    def test_invalid_source_identity_fails_before_the_gates(self):
        self.env['GITHUB_SHA'] = 'invalid'
        result = self.run_step()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.trace.exists())


if __name__ == '__main__':
    unittest.main()
