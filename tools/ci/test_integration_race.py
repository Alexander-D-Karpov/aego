import json
import sys
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest import mock

from integration_race import PACKAGE, check_events, command, groups_for, names_from, run_group, selector


class IntegrationRaceEnumerationTests(unittest.TestCase):
    def event(self, name, action, output=''):
        return {'Package': PACKAGE, 'Test': name, 'Action': action, 'Output': output}

    def test_names_and_groups_cover_every_seed_once(self):
        names = names_from('TestOne\nTestTwo\nExampleValue\nFuzzInput\nok  aego/tests/integration\n')
        groups = groups_for(names)
        self.assertEqual(sorted(name for group in groups for name in group), names)
        self.assertTrue(all(selector(group).startswith('^(') for group in groups))
        with self.assertRaises(ValueError):
            names_from('ok  package\n')
        with self.assertRaises(ValueError):
            names_from('TestOne\nTestOne\n')
        with self.assertRaises(ValueError):
            selector([])

    def test_missing_duplicate_and_unexpected_results_cannot_pass(self):
        expected = ['TestOne', 'TestTwo']
        valid = [self.event(name, action) for name in expected for action in ('run', 'pass')]
        self.assertTrue(check_events(valid, expected)['coverageComplete'])
        self.assertFalse(check_events(valid[:-1], expected)['coverageComplete'])
        self.assertFalse(check_events(valid + [self.event('TestOne', 'pass')], expected)['coverageComplete'])
        self.assertFalse(check_events(valid + [self.event('TestOther', 'run')], expected)['coverageComplete'])

    def test_skips_are_distinct_and_subtest_failures_are_retained(self):
        events = [self.event('TestOne', 'run'), self.event('TestOne/part', 'run'),
                  self.event('TestOne/part', 'output', 'SKIP: unavailable device\n'),
                  self.event('TestOne/part', 'skip'), self.event('TestOne', 'pass')]
        result = check_events(events, ['TestOne'])
        self.assertTrue(result['coverageComplete'])
        self.assertEqual(result['topLevel']['TestOne'], 'PASS')
        self.assertEqual(result['skipped'][0]['acceptance'], 'UNVERIFIED')
        events[3] = self.event('TestOne/part', 'fail')
        self.assertIn('TestOne/part', check_events(events, ['TestOne'])['failures'])

    def test_group_reports_failed_tests_without_accepting_failed_exits(self):
        cases = [
            (0, 'pass', 'PASS'),
            (0, 'skip', 'COMPLETED_WITH_SKIPS'),
            (1, 'fail', 'FAIL'),
            (1, 'pass', 'FAIL'),
            (0, None, 'FAIL'),
        ]
        for exit_code, terminal, status in cases:
            with self.subTest(exit_code=exit_code, terminal=terminal), tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                events = [self.event('TestOne', 'run')]
                if terminal:
                    events += [self.event('TestOne', 'output', 'diagnostic\n'), self.event('TestOne', terminal)]

                def fake_command(argv, cwd, log, **kwargs):
                    self.assertFalse(kwargs['check'])
                    log.write_bytes(b'raw test output\n')
                    log.with_suffix('.exit').write_text(str(exit_code) + '\n')
                    return exit_code

                def fake_converter(argv, **kwargs):
                    kwargs['stdout'].write(b''.join((json.dumps(e) + '\n').encode() for e in events))
                    return SimpleNamespace(returncode=0)

                with mock.patch('integration_race.command', side_effect=fake_command), \
                     mock.patch('integration_race.subprocess.run', side_effect=fake_converter):
                    result = run_group(root / 'integration.test', ['TestOne'], root, root / 'group')
                self.assertEqual(result['status'], status)
                self.assertEqual(result['testExitCode'], exit_code)
                if terminal == 'fail':
                    self.assertEqual(result['failures'], ['TestOne'])
                    self.assertTrue((root / 'group' / 'events.jsonl').is_file())
                if exit_code:
                    self.assertIn('test binary exited', result['error'])

    def test_command_only_allows_failed_exit_when_explicitly_requested(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            argv = [sys.executable, '-c', 'raise SystemExit(7)']
            with self.assertRaises(RuntimeError):
                command(argv, root, root / 'checked.log')
            code = command(argv, root, root / 'unchecked.log', check=False)
            self.assertEqual(code, 7)
            self.assertEqual((root / 'unchecked.exit').read_text(), '7\n')


if __name__ == '__main__':
    unittest.main()
