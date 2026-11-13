"""Check the make-ci final guard without executing the real CI or touching the checkout."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]


class CICleanRootOrderTest(unittest.TestCase):
    def test_clean_root_guard_runs_after_every_prerequisite(self):
        with tempfile.TemporaryDirectory(prefix='aego-ci-order-') as directory:
            root = Path(directory)
            stub_bin = root / 'bin'
            stub_bin.mkdir()
            trace = root / 'trace'
            shell = stub_bin / 'sh'
            shell.write_text('''#!/bin/sh
set -eu
[ "$*" = 'tools/ci-clean-root.sh' ] || exit 91
printf 'clean-root\\n' >> "$AEGO_CI_ORDER_TRACE"
''')
            shell.chmod(0o700)
            extra = root / 'order.mk'
            extra.write_text('''ci: __aego_last_ci_test
.PHONY: __aego_last_ci_test
__aego_last_ci_test:
\t@printf 'last-prerequisite\\n' >> "$$AEGO_CI_ORDER_TRACE"
''')
            env = dict(os.environ, PATH=str(stub_bin) + os.pathsep + os.defpath,
                       HOME=str(root), XDG_CONFIG_HOME=str(root), GOTELEMETRY='off',
                       AEGO_CI_ORDER_TRACE=str(trace))
            for name in ('MAKEFLAGS', 'MFLAGS', 'MAKELEVEL', 'GNUMAKEFLAGS'):
                env.pop(name, None)
            args = ['make', '-f', str(ROOT / 'Makefile'), 'GOOS=linux', 'GOARCH=amd64']
            # The make database gives the expanded prerequisites from every
            # ci: rule, including the module files. -q never runs recipes.
            database = subprocess.run(args + ['-qp', 'ci'], cwd=ROOT, env=env,
                                      stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                      text=True, stdin=subprocess.DEVNULL, timeout=30)
            self.assertIn(database.returncode, (0, 1), database.stderr)
            rows = [line for line in database.stdout.splitlines()
                    if line.startswith('ci: ') and ':=' not in line]
            self.assertEqual(len(rows), 1, rows)
            dependencies = rows[0].split()[1:]
            self.assertIn('site-demo-prepare-test', dependencies)
            self.assertIn('studio-web-test', dependencies)
            skipped = [arg for name in dependencies for arg in ('-o', name)]
            # Mark every actual prerequisite old; a synthetic last prerequisite
            # and a stub for the final guard are the only commands executed.
            result = subprocess.run(args + ['-f', str(extra), *skipped, 'ci'], cwd=ROOT,
                                    env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                                    text=True, stdin=subprocess.DEVNULL, timeout=30)
            self.assertEqual(result.returncode, 0, result.stdout)
            self.assertEqual(trace.read_text().splitlines(), ['last-prerequisite', 'clean-root'], result.stdout)


if __name__ == '__main__':
    unittest.main()
