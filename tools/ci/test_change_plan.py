"""Check conservative change planning; no actual CI gates are run here."""

import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest import mock


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'tools/ci'))
SPEC = importlib.util.spec_from_file_location('aego_change_plan', ROOT / 'tools/ci/change_plan.py')
PLAN = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(PLAN)


class ChangePlanTest(unittest.TestCase):
    def test_non_go_categories_are_combined_without_running_checks(self):
        paths = ['tools/websmoke/try.mjs', 'docs/start.md', 'engine/res/shaders/include/noise.glsl',
                 'tools/artstudio/release/web/studio.js']
        with mock.patch.object(PLAN.affected_packages, 'affected_packages', side_effect=AssertionError('unexpected Go')):
            result = PLAN.plan(ROOT, paths)
        self.assertTrue(result['full_ci_required'])
        self.assertIn('no checks were run', result['scope'])
        self.assertEqual({c['categories'][0] for c in result['changes']}, {'web', 'docs', 'shader', 'studio-web'})
        self.assertTrue({'site-check', 'test-gl', 'studio-web-test', 'web-smoke', 'component-try'} <=
                        set(result['suggested_make_targets']))

    def test_unmapped_assets_and_build_configuration_always_escalate(self):
        for path in ('examples/platformer/assets/hero.png', 'tools/websmoke/package-lock.json',
                     '.github/workflows/ci.yml', 'tools/gendocs/site/static/generated.sh', 'random.file'):
            with self.subTest(path=path):
                self.assertIn('ci-full', PLAN.plan(ROOT, [path])['suggested_make_targets'])

    def test_go_graph_is_separate_from_non_go_advice(self):
        with mock.patch.object(PLAN.affected_packages, 'affected_packages', return_value=['aego/one', 'aego/two']) as graph:
            result = PLAN.plan(ROOT, ['one/file.go', 'docs/start.md'])
        graph.assert_called_once_with(ROOT, ['one/file.go'])
        self.assertEqual(result['host_go_packages'], ['aego/one', 'aego/two'])
        self.assertIn('site-check', result['suggested_make_targets'])
        self.assertTrue(result['full_ci_required'])

    def test_empty_missing_or_incomplete_go_graph_escalates(self):
        for response in ([], ValueError('nested module'), ValueError('incomplete graph'),
                         subprocess.TimeoutExpired('go list', 120)):
            with self.subTest(response=response), mock.patch.object(
                    PLAN.affected_packages, 'affected_packages',
                    **({'side_effect': response} if isinstance(response, Exception) else {'return_value': response})):
                result = PLAN.plan(ROOT, ['new/file.go'])
                self.assertIn('ci-full', result['suggested_make_targets'])
                self.assertTrue(result['host_go_graph_error'])

    def test_invalid_paths_do_not_produce_a_plan(self):
        for path in ('', '../outside.go', '/absolute.go', 'one/../two.go', 'one//two.go'):
            with self.subTest(path=path), self.assertRaises(ValueError):
                PLAN.plan(ROOT, [path])

    def test_all_suggested_targets_exist_in_make_database(self):
        with tempfile.TemporaryDirectory(prefix='aego-plan-targets-') as scratch:
            env = dict(os.environ, HOME=scratch, XDG_CONFIG_HOME=scratch, GOTELEMETRY='off')
            for key in ('MAKEFLAGS', 'MFLAGS', 'MAKELEVEL', 'GNUMAKEFLAGS'):
                env.pop(key, None)
            result = subprocess.run(['make', '-qp', '-f', str(ROOT / 'Makefile'),
                                     'GOOS=linux', 'GOARCH=amd64', 'XVFB=', 'ci'], cwd=ROOT,
                                    env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=30)
        self.assertIn(result.returncode, (0, 1), result.stderr)
        for targets in PLAN.RULES.values():
            for target in targets:
                self.assertRegex(result.stdout, r'(?m)^' + target + r':(?:\s|$)')

    def test_missing_revision_fails_instead_of_emitting_empty_success(self):
        with tempfile.TemporaryDirectory(prefix='aego-plan-repo-') as scratch:
            repo = Path(scratch)
            subprocess.run(['git', 'init', '-q', str(repo)], check=True, timeout=10)
            result = subprocess.run([sys.executable, '-B', str(ROOT / 'tools/ci/change_plan.py'),
                                     '--base=missing-branch'], cwd=repo, text=True, stdout=subprocess.PIPE,
                                    stderr=subprocess.PIPE, timeout=15)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(result.stdout, '')
        self.assertIn('change plan failed', result.stderr)

    def test_real_git_rename_untracked_and_go_change_are_all_reported(self):
        with tempfile.TemporaryDirectory(prefix='aego-plan-git-') as scratch:
            root = Path(scratch)
            repo, fakebin = root / 'repo', root / 'bin'
            repo.mkdir()
            fakebin.mkdir()
            fakego = fakebin / 'go'
            fakego.write_text('''#!/bin/sh
case "$*" in
  'list -m') printf 'aego\\n' ;;
  'list -deps -test -json ./...') printf '%s\\n' '{"ImportPath":"aego/pkg"}' ;;
  *) exit 91 ;;
esac
''')
            fakego.chmod(0o755)
            env = dict(os.environ, PATH=str(fakebin) + os.pathsep + os.defpath,
                       HOME=str(root), GIT_CONFIG_GLOBAL=os.devnull, GIT_CONFIG_NOSYSTEM='1',
                       GIT_AUTHOR_NAME='Fixture', GIT_AUTHOR_EMAIL='fixture@example.invalid',
                       GIT_COMMITTER_NAME='Fixture', GIT_COMMITTER_EMAIL='fixture@example.invalid')
            for key in ('GIT_DIR', 'GIT_WORK_TREE', 'GIT_INDEX_FILE'):
                env.pop(key, None)

            def git(*args):
                return subprocess.check_output(['git', *args], cwd=repo, env=env, text=True, timeout=10).strip()

            git('init', '-q')
            (repo / 'docs').mkdir()
            (repo / 'pkg').mkdir()
            (repo / 'docs/old.md').write_text('fixture\n')
            (repo / 'pkg/code.go').write_text('package pkg\n')
            git('add', '.')
            git('-c', 'commit.gpgsign=false', 'commit', '-qm', 'fixture')
            base = git('rev-parse', 'HEAD')
            git('mv', 'docs/old.md', 'docs/new.md')
            (repo / 'pkg/code.go').write_text('package pkg\n// changed\n')
            (repo / 'new.glsl').write_text('fixture\n')
            result = subprocess.run([sys.executable, '-B', str(ROOT / 'tools/ci/change_plan.py'), '--base=' + base],
                                    cwd=repo, env=env, text=True, stdout=subprocess.PIPE,
                                    stderr=subprocess.PIPE, timeout=15)
            self.assertEqual(result.returncode, 0, result.stderr)
            plan = json.loads(result.stdout)
            self.assertEqual([item['path'] for item in plan['changes']],
                             ['docs/new.md', 'docs/old.md', 'new.glsl', 'pkg/code.go'])
            self.assertEqual(plan['host_go_packages'], ['aego/pkg'])
            self.assertTrue({'site-check', 'shader-validator', 'test-gl'} <= set(plan['suggested_make_targets']))
            self.assertTrue(plan['full_ci_required'])


if __name__ == '__main__':
    unittest.main()
