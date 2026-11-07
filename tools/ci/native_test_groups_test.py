import types
import unittest
from pathlib import Path

import native_test_groups as native


class NativeTestGroups(unittest.TestCase):
    def test_keeps_every_test_example_and_fuzz_once_in_order(self):
        names = native.tests("TestAlpha\nTestBeta\nFuzzGamma\nExampleDelta\nok  aego/tests/integration  0.02s\n")
        parts = native.groups(names, 3)
        self.assertEqual([n for part in parts for n in part], names)
        for name in names:
            self.assertEqual(sum(name in part for part in parts), 1)

    def test_rejects_missing_or_duplicate_inventory(self):
        for output in ("", "TestAlpha\nTestAlpha\n", "TestInvalid name\n"):
            with self.subTest(output=output), self.assertRaises(ValueError):
                native.tests(output)
        for output in ("aego/app\n", "aego/tests/integration\n", "aego/tests/integration\naego/tests/integration\n"):
            with self.subTest(output=output), self.assertRaises(ValueError):
                native.packages(output)

    def test_escapes_exact_names_and_anchors_the_pattern(self):
        self.assertEqual(native.test_pattern(["TestA", "TestAB"]), "^(?:TestA|TestAB)$")

    def test_runs_all_groups_even_after_failure_and_propagates_it(self):
        calls = []

        def output(args):
            if args[1] == "list":
                return "aego/app\naego/tests/integration\naego/ui\n"
            if args[3] != "^(Test|Example|Fuzz)":
                name = args[3][4:-2]
                return name + "\nok  aego/tests/integration  0.02s\n"
            return "TestA\nTestB\nTestC\nTestD\nok  aego/tests/integration  0.02s\n"

        def execute(args):
            calls.append(args)
            return types.SimpleNamespace(returncode=1 if len(calls) == 2 else 0)

        self.assertEqual(native.run(execute=execute, output=output), 1)
        self.assertEqual(len(calls), 5)
        self.assertEqual(calls[0], ["go", "test", "-count=1", "aego/app", "aego/ui"])
        self.assertEqual([c[-1] for c in calls[1:]], ["./tests/integration"] * 4)
        self.assertEqual([c[-2] for c in calls[1:]], ["^(?:TestA)$", "^(?:TestB)$", "^(?:TestC)$", "^(?:TestD)$"])

    def test_fails_before_execution_when_go_matches_a_different_set(self):
        def output(args):
            return "TestOther\nok  aego/tests/integration  0.02s\n"

        with self.assertRaises(ValueError):
            native.verify_patterns([["TestExpected"]], ["^(?:TestExpected)$"], output)

    def test_installer_diagnostics_do_not_allow_failed_uploads(self):
        workflow = (Path(__file__).resolve().parents[2] / ".github/workflows/github-native.yml").read_text(encoding="utf8")
        package = workflow.split("- name: Build Windows portable and installer", 1)[1].split("- name: Native silent installer", 1)[0]
        smoke = workflow.split("- name: Native silent installer and uninstaller", 1)[1].split("- name: Build macOS", 1)[0]
        uploads = workflow.split("- name: Upload Windows portable", 1)[1].split("- name: Upload macOS", 1)[0]
        self.assertIn("success() || failure()", package)
        self.assertIn("steps.native_sdl.conclusion == 'success'", package)
        self.assertIn("steps.native_build.conclusion == 'success'", package)
        self.assertIn("steps.windows_package.conclusion == 'success'", smoke)
        self.assertNotIn("continue-on-error", package + smoke)
        self.assertEqual(uploads.count("runner.os == 'Windows' && success()"), 2)


if __name__ == "__main__":
    unittest.main()
