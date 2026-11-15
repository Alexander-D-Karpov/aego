"""Check the affected selector in disposable Git repositories with a fake Go graph."""

import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
SELECTOR = ROOT / 'tools/ci/affected_packages.py'


class AffectedPackagesTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='aego-affected-test-')
        self.addCleanup(self.temp.cleanup)
        self.scratch = Path(self.temp.name)
        self.repo = self.scratch / 'repo'
        self.repo.mkdir()
        self.bin = self.scratch / 'bin'
        self.bin.mkdir()
        self.metadata = self.scratch / 'go-list.json'
        packages = [
            {'ImportPath': 'aego/alpha'},
            {'ImportPath': 'aego/beta', 'Deps': ['aego/alpha']},
            {'ImportPath': 'aego/helper', 'Imports': ['aego/beta']},
            {'ImportPath': 'aego/gamma', 'TestImports': ['aego/helper']},
            {'ImportPath': 'aego/testconsumer [aego/testconsumer.test]', 'ForTest': 'aego/testconsumer',
             'Deps': ['aego/alpha [aego/testconsumer.test]']},
        ]
        self.metadata.write_text('\n'.join(json.dumps(x) for x in packages))
        go = self.bin / 'go'
        go.write_text('''#!/bin/sh
case "$*" in
  'list -m') printf 'aego\\n' ;;
  'list -deps -test -json ./...')
    [ "${GO_GRAPH_FAIL:-0}" = 0 ] || { printf 'graph failed\\n' >&2; exit 42; }
    cat "$MOCK_GO_METADATA" ;;
  *) exit 91 ;;
esac
''')
        go.chmod(0o755)
        self.env = dict(os.environ, PATH=str(self.bin) + os.pathsep + os.defpath,
                        HOME=str(self.scratch), GIT_CONFIG_GLOBAL=os.devnull,
                        GIT_CONFIG_NOSYSTEM='1', GIT_AUTHOR_NAME='Fixture', GIT_AUTHOR_EMAIL='fixture@example.invalid',
                        GIT_COMMITTER_NAME='Fixture', GIT_COMMITTER_EMAIL='fixture@example.invalid',
                        MOCK_GO_METADATA=str(self.metadata))
        for name in ('GIT_DIR', 'GIT_WORK_TREE', 'GIT_INDEX_FILE', 'GO_GRAPH_FAIL'):
            self.env.pop(name, None)
        for name, content in {
            'go.mod': 'module aego\ngo 1.27\n',
            'alpha/a.go': 'package alpha\nconst X = 1\n',
            'beta/b.go': 'package beta\nimport "aego/alpha"\nvar X = alpha.X\n',
            'gamma/c.go': 'package gamma\n',
            'gamma/c_test.go': 'package gamma\nimport ("testing"; "aego/beta")\nfunc TestValue(t *testing.T) { if beta.X != 1 { t.Fatal(beta.X) } }\n',
            'tools/nested/go.mod': 'module nested\ngo 1.27\n',
            'tools/nested/n.go': 'package nested\n',
            'README.md': 'fixture\n',
        }.items():
            path = self.repo / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(content)
        self.git('init', '-q')
        self.git('add', '.')
        self.git('-c', 'commit.gpgsign=false', 'commit', '-q', '-m', 'fixture')
        self.base = self.git('rev-parse', 'HEAD').strip()

    def git(self, *args, cwd=None):
        return subprocess.check_output(['git', *args], cwd=cwd or self.repo, env=self.env,
                                       stderr=subprocess.PIPE, text=True, timeout=10)

    def select(self, base=None, cwd=None, **env):
        return subprocess.run([sys.executable, '-B', str(SELECTOR), base or self.base],
                              cwd=cwd or self.repo, env=dict(self.env, **env), text=True,
                              stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=15)

    def test_clean_tree_returns_no_packages(self):
        result = self.select()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, '')

    def test_go_change_includes_transitive_and_test_consumers(self):
        (self.repo / 'alpha/a.go').write_text('package alpha\n// edit\n')
        result = self.select()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.splitlines(), [
            'aego/alpha', 'aego/beta', 'aego/gamma', 'aego/helper', 'aego/testconsumer'])
        self.assertIn('full CI gates remain required', result.stderr)

    def test_staged_rename_and_untracked_go_are_detected(self):
        self.git('mv', 'alpha/a.go', 'alpha/renamed.go')
        (self.repo / 'beta/new.go').write_text('package beta\n')
        result = self.select()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('aego/alpha', result.stdout.splitlines())
        self.assertIn('aego/beta', result.stdout.splitlines())

    def test_non_go_nested_module_deleted_package_or_missing_base_fail(self):
        (self.repo / 'shader.glsl').write_text('fixture')
        result = self.select()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('non-Go changes', result.stderr)
        (self.repo / 'shader.glsl').unlink()
        (self.repo / 'tools/nested/n.go').write_text('package nested\n// edit\n')
        result = self.select()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('nested Go module', result.stderr)
        self.git('restore', 'tools/nested/n.go')
        self.git('rm', '-q', 'alpha/a.go')
        result = self.select()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('deleted package', result.stderr)
        result = self.select('missing-branch')
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(result.stdout, '')

    def test_graph_failure_is_not_an_empty_success(self):
        (self.repo / 'alpha/a.go').write_text('package alpha\n// edit\n')
        result = self.select(GO_GRAPH_FAIL='1')
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(result.stdout, '')
        self.assertIn('graph failed', result.stderr)

    def test_real_go_graph_includes_reverse_test_imports(self):
        go = shutil.which('go')
        if not go:
            self.skipTest('Go is unavailable')
        (self.repo / 'alpha/a.go').write_text('package alpha\nconst X = 1\n// edit\n')
        env = dict(self.env, PATH=str(Path(go).parent) + os.pathsep + os.defpath,
                   GOCACHE=str(self.scratch / 'gocache'), GOMODCACHE=str(self.scratch / 'gomodcache'),
                   GOPROXY='off', GOSUMDB='off', GOTOOLCHAIN='local', GOTELEMETRY='off')
        result = subprocess.run([sys.executable, '-B', str(SELECTOR), self.base], cwd=self.repo, env=env,
                                text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=90)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.splitlines(), ['aego/alpha', 'aego/beta', 'aego/gamma'])

    def test_shallow_checkout_is_rejected(self):
        clone = self.scratch / 'shallow'
        subprocess.check_call(['git', 'clone', '-q', '--depth=1', 'file://' + str(self.repo), str(clone)],
                              env=self.env, timeout=15)
        result = self.select('HEAD', cwd=clone)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('shallow checkout', result.stderr)

    def test_make_targets_propagate_selector_failure(self):
        shell = self.bin / 'sh'
        shell.write_text('#!/bin/sh\nprintf "selector fixture failed\\n" >&2\nexit 73\n')
        shell.chmod(0o755)
        for target in ('affected', 'quick'):
            result = subprocess.run(['make', '-f', str(ROOT / 'Makefile'), 'GOOS=linux', 'GOARCH=amd64', target],
                                    cwd=ROOT, env=self.env, text=True, stdout=subprocess.PIPE,
                                    stderr=subprocess.STDOUT, timeout=15)
            with self.subTest(target=target):
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertIn('selector fixture failed', result.stdout)
                self.assertNotIn('nothing affected', result.stdout)

    def test_make_targets_pass_base_as_data_and_require_fresh_tests(self):
        shell = self.bin / 'sh'
        trace = self.scratch / 'base-argument'
        shell.write_text('#!/bin/sh\nprintf "%s\\n" "$2" > "$BASE_TRACE"\nexit 73\n')
        shell.chmod(0o755)
        payload = 'origin/master; touch ' + str(self.scratch / 'injected')
        env = dict(self.env, BASE_TRACE=str(trace))
        for target in ('affected', 'quick'):
            result = subprocess.run(['make', '-f', str(ROOT / 'Makefile'), 'GOOS=linux', 'GOARCH=amd64',
                                     target, 'BASE=' + payload], cwd=ROOT, env=env, text=True,
                                    stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=15)
            with self.subTest(target=target):
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertEqual(trace.read_text().strip(), payload)
                self.assertFalse((self.scratch / 'injected').exists())
        makefile = (ROOT / 'Makefile').read_text()
        for marker in ('go test -count=1 $$pkgs', 'go test -count=1 -short $$pkgs'):
            self.assertIn(marker, makefile)


if __name__ == '__main__':
    unittest.main()
