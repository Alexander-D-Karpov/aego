"""Exercise the actual CI wrapper tail without running the full CI."""

import os
from pathlib import Path
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / 'tools/acceptance-check.sh'


def wrapper_tail():
    text = SCRIPT.read_text(encoding='utf-8')
    marker = 'if [[ "$mode" == ci ]]; then\n'
    if text.count(marker) != 1:
        raise ValueError('CI wrapper tail is missing or ambiguous')
    return marker + text.split(marker, 1)[1].split('\nprintf ', 1)[0]


class AcceptanceCIWrapperTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='aego-acceptance-ci-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        self.trace = self.root / 'trace'
        self.stub('make', '''#!/bin/sh
printf 'make|%s\\n' "$*" >> "$TRACE"
exit "${MAKE_EXIT:-0}"
''')
        self.stub('acceptance-check', '''#!/bin/sh
printf 'try|%s\\n' "$*" >> "$TRACE"
exit "${TRY_EXIT:-0}"
''')
        self.env = dict(os.environ, PATH=str(self.bin) + os.pathsep + os.defpath, TRACE=str(self.trace),
                        FIXTURE_ROOT=str(self.root))
        for name in ('MAKE_EXIT', 'TRY_EXIT'):
            self.env.pop(name, None)

    def stub(self, name, text):
        path = self.bin / name
        path.write_text(text)
        path.chmod(0o755)

    def run_wrapper(self):
        code = 'set -euo pipefail\nroot="$FIXTURE_ROOT"\nmode=ci\n' + wrapper_tail()
        return subprocess.run(['bash', '-c', code, str(self.bin / 'acceptance-check')], cwd=self.root, env=self.env, text=True,
                              stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=10)

    def calls(self):
        return self.trace.read_text().splitlines() if self.trace.exists() else []

    def test_runner_remains_executable_for_its_recursive_try_call(self):
        self.assertTrue(os.access(SCRIPT, os.X_OK), str(SCRIPT))

    def test_success_preserves_the_separate_try_gate(self):
        result = self.run_wrapper()
        self.assertEqual(result.returncode, 0, result.stdout)
        self.assertEqual(self.calls(), ['make|ci', 'try|try'])

    def test_make_or_try_failure_is_propagated(self):
        self.env['MAKE_EXIT'] = '19'
        result = self.run_wrapper()
        self.assertEqual(result.returncode, 19, result.stdout)
        self.assertEqual(self.calls(), ['make|ci'])
        self.trace.unlink()
        self.env['MAKE_EXIT'] = '0'
        self.env['TRY_EXIT'] = '23'
        result = self.run_wrapper()
        self.assertEqual(result.returncode, 23, result.stdout)
        self.assertEqual(self.calls(), ['make|ci', 'try|try'])


if __name__ == '__main__':
    unittest.main()
