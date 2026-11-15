"""Exercise the real site/playground Make graph with isolated command stubs."""

import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]


class SiteBuildGraphTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='aego-site-graph-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        shutil.copyfile(ROOT / 'Makefile', self.root / 'Makefile')
        self.write('tools/artstudio/release/release.mk', '')
        self.write('third_party/sdl/version.txt', 'version=3.2.0\n')
        self.write('tools/websmoke/.keep', '')
        self.trace = self.root / 'trace'
        self.write('bin/go', '''#!/bin/sh
set -eu
case "$*" in
  'list -deps ./cmd/aego-playground')
    [ "$GOOS" = js ] && [ "$GOARCH" = wasm ] || exit 90
    printf 'guard\\n' >> "$TRACE"
    case "${GRAPH_MODE:-good}" in
      good) printf 'aego/core\\naego/cmd/aego-playground\\n' ;;
      forbidden) printf 'aego/platform/sdl\\naego/cmd/aego-playground\\n' ;;
      empty) : ;;
      error) printf 'list failed\\n' >&2; exit 74 ;;
      *) exit 91 ;;
    esac ;;
  'build -o .cache/bin/aego ./cmd/aego') printf 'cli\\n' >> "$TRACE" ;;
  *) printf 'unexpected go command: %s\\n' "$*" >&2; exit 92 ;;
esac
''', executable=True)
        self.write('.cache/bin/aego', '''#!/bin/sh
set -eu
case "$*" in
  'docs build '*)
    printf 'site\\n' >> "$TRACE"
    mkdir -p site/play site/try
    printf 'wasm' > site/play/aego.wasm.gz
    printf 'wasm' > site/try/aego-docs.wasm.gz ;;
  *) printf 'unexpected aego command: %s\\n' "$*" >&2; exit 93 ;;
esac
''', executable=True)
        self.write('demo-export', '''#!/bin/sh
set -eu
printf 'demo-export\\n' >> "$TRACE"
mkdir -p site/demo
printf 'wasm' > site/demo/game.wasm.gz
''', executable=True)
        self.write('tools/webbudget.sh', '#!/bin/sh\nprintf "webbudget\\n" >> "$TRACE"\n')
        self.write('bin/npm', '#!/bin/sh\nprintf "npm %s\\n" "$*" >> "$TRACE"\n', executable=True)
        self.write('tools/ci/prepared_web_deps.py', '''import os
from pathlib import Path
with Path(os.environ['TRACE']).open('a') as out:
    out.write('prepared-web-check\\n')
''')
        self.write('tools/acceptance-check.sh', '#!/bin/sh\nprintf "acceptance %s\\n" "$*" >> "$TRACE"\n')
        self.env = dict(os.environ, PATH=str(self.root / 'bin') + os.pathsep + os.defpath,
                        TRACE=str(self.trace), HOME=str(self.root), XDG_CONFIG_HOME=str(self.root),
                        GOTELEMETRY='off')
        for name in ('MAKEFLAGS', 'MFLAGS', 'MAKELEVEL', 'GNUMAKEFLAGS', 'GRAPH_MODE', 'AEGO_CI_PREPARED'):
            self.env.pop(name, None)

    def write(self, name, contents, executable=False):
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(contents, encoding='utf-8')
        if executable:
            path.chmod(0o700)

    def run_make(self, *targets, graph_mode='good', parallel=False, prepared='0'):
        env = dict(self.env, GRAPH_MODE=graph_mode)
        env['AEGO_CI_PREPARED'] = prepared
        args = ['make', '-f', 'Makefile', '-o', 'bootstrap', 'GOOS=linux', 'GOARCH=amd64', 'XVFB=',
                'WEB_DEMO_EXPORT=sh demo-export', 'CHROMIUM=fixture',
                'WEB_SMOKE=true', 'WEB_DEMO=true', 'WEB_TRY=true', 'WEB_AUDIO=true',
                'WEB_THEME=true', 'WEB_BENCH=true']
        if parallel:
            args.append('-j4')
        result = subprocess.run(args + list(targets), cwd=self.root, env=env, text=True,
                                stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                                stdin=subprocess.DEVNULL, timeout=15)
        trace = self.trace.read_text().splitlines() if self.trace.exists() else []
        return result, trace

    def assert_one_site(self, result, trace):
        self.assertEqual(result.returncode, 0, result.stdout)
        for entry in ('guard', 'cli', 'site', 'demo-export'):
            self.assertEqual(trace.count(entry), 1, (trace, result.stdout))
        self.assertLess(trace.index('guard'), trace.index('site'))
        self.assertLess(trace.index('site'), trace.index('demo-export'))

    def test_individual_entry_points_build_a_guarded_site(self):
        for target in ('playground', 'site', 'web-smoke', 'component-try'):
            with self.subTest(target=target):
                if self.trace.exists():
                    self.trace.unlink()
                result, trace = self.run_make(target)
                self.assert_one_site(result, trace)
                if target == 'web-smoke':
                    self.assertIn('webbudget', trace)
                if target == 'component-try':
                    self.assertIn('acceptance component-try', trace)
                expected_installs = 1 if target in ('web-smoke', 'component-try') else 0
                self.assertEqual(trace.count('npm ci --no-audit --no-fund'), expected_installs, trace)

    def test_multiple_consumers_share_one_site_in_serial_and_parallel_make(self):
        for parallel in (False, True):
            with self.subTest(parallel=parallel):
                if self.trace.exists():
                    self.trace.unlink()
                result, trace = self.run_make('playground', 'web-smoke', 'component-try', parallel=parallel)
                self.assert_one_site(result, trace)
                self.assertIn('webbudget', trace)
                self.assertIn('acceptance component-try', trace)
                self.assertEqual(trace.count('npm ci --no-audit --no-fund'), 1, trace)

    def test_all_browser_consumers_share_the_web_dependency_target(self):
        targets = ('web-smoke', 'web-smoke-update', 'component-try', 'web-bench', 'web-bench-baseline',
                   'web-bench-stage12', 'web-bench-platformer', 'web-materials', 'web-gpu-smoke',
                   'web-lighting', 'web-particle-goldens', 'stage12-release-check')
        result = subprocess.run(['make', '-rR', '-f', 'Makefile', '-qp', '-o', 'bootstrap', 'GOOS=linux', 'GOARCH=amd64',
                                 'XVFB=', *targets], cwd=self.root, env=self.env, text=True,
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                stdin=subprocess.DEVNULL, timeout=15)
        self.assertIn(result.returncode, (0, 1), result.stderr)
        self.assertFalse(result.stderr, result.stderr)
        for target in targets:
            with self.subTest(target=target):
                rows = re.findall(r'^' + re.escape(target) + r':\s*([^\n]*)$', result.stdout, re.MULTILINE)
                self.assertEqual(len(rows), 1, rows)
                self.assertIn('web-deps', rows[0].split())

    def test_prepared_mode_checks_once_and_does_not_run_npm(self):
        result, trace = self.run_make('playground', 'web-smoke', 'component-try', prepared='1')
        self.assert_one_site(result, trace)
        self.assertEqual(trace.count('prepared-web-check'), 1, trace)
        self.assertFalse(any(item.startswith('npm ') for item in trace), trace)

    def test_unrecognized_prepared_mode_fails_before_npm(self):
        result, trace = self.run_make('web-smoke', prepared='invalid')
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertFalse(any(item.startswith('npm ') for item in trace), trace)

    def test_invalid_or_unavailable_graph_never_builds_the_site(self):
        for mode in ('forbidden', 'error', 'empty'):
            for target in ('playground', 'web-smoke', 'component-try'):
                with self.subTest(mode=mode, target=target):
                    if self.trace.exists():
                        self.trace.unlink()
                    result, trace = self.run_make(target, graph_mode=mode)
                    self.assertNotEqual(result.returncode, 0, result.stdout)
                    self.assertIn('guard', trace)
                    self.assertNotIn('site', trace)
                    self.assertNotIn('demo-export', trace)
                    self.assertNotIn('acceptance component-try', trace)


if __name__ == '__main__':
    unittest.main()
