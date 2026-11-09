import unittest

from integration_race import PACKAGE, check_events, groups_for, names_from, selector


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


if __name__ == '__main__':
    unittest.main()
