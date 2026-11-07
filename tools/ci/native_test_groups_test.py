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

    def test_optional_stage_art_checks_do_not_replace_native_ci(self):
        workflow = (Path(__file__).resolve().parents[2] / ".github/workflows/github-native.yml").read_text(encoding="utf8")
        inputs = workflow.split("stage_art_checks:", 1)[1].split("permissions:", 1)[0]
        self.assertIn("default: false", inputs)
        self.assertIn("type: boolean", inputs)
        native_job = workflow.split("  native:", 1)[1].split("  stage-remote-store:", 1)[0]
        self.assertIn("steps.native_build.conclusion == 'success'", native_job)
        stage = workflow.split("  stage-remote-store:", 1)[1].split("  gitea-result:", 1)[0]
        self.assertIn('test "$(git rev-parse HEAD)" = "$STAGE_COMMIT"', stage)
        art = stage.split("- name: Test Stage Aseprite import without a GUI", 1)[1]
        self.assertIn("if: ${{ inputs.stage_art_checks }}", art)
        for name in ("TestRealAsepriteFilesMatchLibreSpriteAndRoundTrip", "TestAsepriteImportCommandOpensEditableDocumentAndPreservesSource", "TestReadExternalFileIsBoundedAndRegular"):
            self.assertIn(f"-list '^{name}$'", art)
            self.assertIn(f"grep -Fx {name}", art)
        self.assertNotIn("continue-on-error", art)

    def test_installer_diagnostic_creates_its_own_guarded_project(self):
        smoke = (Path(__file__).resolve().parent / "github-windows-installer-smoke.ps1").read_text(encoding="utf8")
        self.assertIn("$installedProject = Join-Path $env:RUNNER_TEMP 'aego-ci-installed-project'", smoke)
        self.assertIn("if (Test-Path $installedProject) { throw", smoke)
        self.assertNotIn(".cache\\github-ci\\project", smoke)
        steps = ("new $installedProject", "import $installedProject", "check $installedProject", "run $installedProject --headless --frames 60")
        positions = [smoke.index("& $installedCLI " + step) for step in steps]
        self.assertEqual(positions, sorted(positions))
        for i, pos in enumerate(positions):
            end = positions[i + 1] if i + 1 < len(positions) else smoke.index("$userData =", pos)
            self.assertIn("if ($LASTEXITCODE -ne 0) { throw", smoke[pos:end])


if __name__ == "__main__":
    unittest.main()
