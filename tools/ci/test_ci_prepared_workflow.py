"""Keep prepared CI jobs pinned, validated and equivalent to the full gates."""

from pathlib import Path
import re
import unittest


ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = ROOT / '.github/workflows/ci.yml'


def job(name, next_name):
    text = WORKFLOW.read_text(encoding='utf-8')
    return text.split('\n  ' + name + ':\n', 1)[1].split('\n  ' + next_name + ':\n', 1)[0]


class PreparedWorkflowTest(unittest.TestCase):
    def test_each_linux_job_pins_and_checks_the_same_prepared_image(self):
        images = []
        for name, next_name in (('checks', 'native-authoring'),
                                ('native-authoring', 'build'), ('build', 'release')):
            with self.subTest(job=name):
                body = job(name, next_name)
                matches = re.findall(r'(?m)^\s+image: (git\.akarpov\.ru/sanspie/aego-ci-env@sha256:[0-9a-f]{64})$', body)
                self.assertEqual(len(matches), 1)
                images.extend(matches)
                self.assertIn('shell: bash', body)
                self.assertIn('AEGO_CI_PREPARED: "1"', body)
                self.assertIn('ARTSTUDIO_PYTHON: /opt/aego/python/bin/python', body)
                self.assertIn('PLAYWRIGHT_BROWSERS_PATH: /ms-playwright', body)
                self.assertNotIn('uses: actions/setup-go@v5', body)
                self.assertNotIn('uses: actions/setup-node@v4', body)
                self.assertNotIn('sudo apt-get', body)
                positions = [body.index(value) for value in (
                    'actions/checkout@v4', 'tools/ci/prepare_ci_workspace.py',
                    'tools/ci/env/preflight.py', 'bash tools/check-glslang.sh',
                    'bash tools/bootstrap-native.sh', 'name: Restore Go build cache')]
                self.assertEqual(positions, sorted(positions))
        self.assertEqual(len(set(images)), 1)

    def test_prepared_browser_paths_do_not_override_the_job_environment(self):
        checks = job('checks', 'native-authoring')
        native = job('native-authoring', 'build')
        self.assertNotIn('PLAYWRIGHT_BROWSERS_PATH: ${{ runner.temp }}', checks + native)
        self.assertNotIn('export PLAYWRIGHT_BROWSERS_PATH="$RUNNER_TEMP/', checks + native)
        self.assertIn('ARTSTUDIO_PYTHON', checks)
        self.assertIn('ARTSTUDIO_PYTHON', native)
        for gate in ('make ci XVFB=', 'component_try_evidence.py verify'):
            self.assertIn(gate, checks)
        for gate in ('bash tools/acceptance-check.sh artstudio-native', 'file_interop_e2e.py',
                     'make remote-examples-check', 'make nsis-check'):
            self.assertIn(gate, native)


if __name__ == '__main__':
    unittest.main()
