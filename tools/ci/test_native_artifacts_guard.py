"""Offline checks for the manual, non-publishing artifact gate."""

import copy
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import textwrap
import unittest
from unittest import mock

import native_artifacts_guard as guard


SHA = "a" * 40
SOURCE = 'package engine\nvar Version = "0.12.5"\n'
ENV = {
    "GITHUB_ACTIONS": "true",
    "GITHUB_SERVER_URL": "https://github.com",
    "GITHUB_REPOSITORY": "Alexander-D-Karpov/aego",
    "GITHUB_EVENT_NAME": "workflow_dispatch",
    "GITHUB_WORKFLOW": "aego-native-artifacts-only",
    "GITHUB_REF": "refs/heads/master",
    "RUNNER_ENVIRONMENT": "github-hosted",
    "GITHUB_SHA": SHA,
    "AEGO_CI_PACKAGE_VERSION": "0.12.5",
    "AEGO_CI_PACKAGE_COMMIT": SHA,
    "AEGO_GITEA_RUN_ID": "486",
}
RUN = {
    "link": "/sanspie/aego/actions/runs/486",
    "workflowID": "ci.yml",
    "triggerEvent": "push",
    "done": True,
    "status": "success",
    "commit": {
        "link": "/sanspie/aego/commit/" + SHA,
        "branch": {"name": "master", "isDeleted": False},
    },
    "jobs": [
        {"jobId": "checks", "status": "success"},
        {"jobId": "native-authoring", "status": "success"},
        {"jobId": "build", "status": "success"},
        {"jobId": "release", "status": "skipped"},
    ],
}


class ContextTest(unittest.TestCase):
    def test_accepts_matching_manual_context(self):
        self.assertEqual(guard.validate_context(ENV, SOURCE, SHA), ("0.12.5", SHA, "486"))
        self.assertEqual(guard.validate_context(ENV, SOURCE.replace("\n", "\r\n"), SHA)[0], "0.12.5")

    def test_rejects_incorrect_or_missing_environment(self):
        for key in ENV:
            for value in (None, "wrong"):
                with self.subTest(key=key, value=value):
                    env = ENV.copy()
                    if value is None:
                        env.pop(key)
                    else:
                        env[key] = value
                    with self.assertRaises(guard.GuardError):
                        guard.validate_context(env, SOURCE, SHA)

    def test_rejects_unsafe_versions_and_run_ids(self):
        for version in ("0.12.5-dev", "v0.12.5", "0.012.5", "65536.0.0", "1.2.3;echo x", ""):
            with self.subTest(version=version):
                with self.assertRaises(guard.GuardError):
                    guard.validate_context({**ENV, "AEGO_CI_PACKAGE_VERSION": version}, SOURCE, SHA)
        for run_id in ("0", "-1", "01", "486/../../x", "12345678901", ""):
            with self.subTest(run_id=run_id):
                with self.assertRaises(guard.GuardError):
                    guard.validate_context({**ENV, "AEGO_GITEA_RUN_ID": run_id}, SOURCE, SHA)

    def test_requires_exact_source_version_and_checkout(self):
        for source in ("", SOURCE.replace("0.12.5", "0.12.5-dev"), SOURCE + SOURCE):
            with self.subTest(source=source):
                with self.assertRaises(guard.GuardError):
                    guard.validate_context(ENV, source, SHA)
        with self.assertRaises(guard.GuardError):
            guard.validate_context(ENV, SOURCE, "b" * 40)


class GiteaRunTest(unittest.TestCase):
    def test_accepts_one_complete_matching_run(self):
        guard.validate_gitea_run(RUN, "486", SHA)

    def test_rejects_bad_identity_or_incomplete_run(self):
        changes = [
            ("link", "/other"), ("workflowID", "other.yml"),
            ("triggerEvent", "pull_request"), ("done", False),
            ("status", "failure"), ("jobs", []), ("jobs", None),
        ]
        for key, value in changes:
            with self.subTest(key=key, value=value):
                run = copy.deepcopy(RUN)
                run[key] = value
                with self.assertRaises(guard.GuardError):
                    guard.validate_gitea_run(run, "486", SHA)

    def test_rejects_changed_commit_or_branch(self):
        for key, value in (("link", "/wrong"), ("branch", None),
                           ("branch", {"name": "other", "isDeleted": False}),
                           ("branch", {"name": "master", "isDeleted": True})):
            with self.subTest(key=key, value=value):
                run = copy.deepcopy(RUN)
                run["commit"][key] = value
                with self.assertRaises(guard.GuardError):
                    guard.validate_gitea_run(run, "486", SHA)

    def test_each_required_job_must_pass_once(self):
        for job_id in sorted(guard.REQUIRED_JOBS):
            with self.subTest(job=job_id, reason="failed"):
                run = copy.deepcopy(RUN)
                next(job for job in run["jobs"] if job["jobId"] == job_id)["status"] = "failure"
                with self.assertRaises(guard.GuardError):
                    guard.validate_gitea_run(run, "486", SHA)
            with self.subTest(job=job_id, reason="duplicate"):
                run = copy.deepcopy(RUN)
                run["jobs"].append({"jobId": job_id, "status": "success"})
                with self.assertRaises(guard.GuardError):
                    guard.validate_gitea_run(run, "486", SHA)


class GateTest(unittest.TestCase):
    def test_requires_refs_before_and_after_green_ci(self):
        with mock.patch.object(guard, "fetch_run", return_value=RUN), \
             mock.patch.object(guard, "check_refs") as refs, \
             mock.patch.object(guard.mirror_releases, "ci_ready", return_value=True):
            guard.require_ci("486", SHA)
            self.assertEqual(refs.call_args_list, [mock.call(SHA), mock.call(SHA)])

    def test_rejects_failed_ci_or_ref_change(self):
        with mock.patch.object(guard, "fetch_run", return_value=RUN), \
             mock.patch.object(guard, "check_refs"), \
             mock.patch.object(guard.mirror_releases, "ci_ready", return_value=False):
            with self.assertRaises(guard.GuardError):
                guard.require_ci("486", SHA)
        with mock.patch.object(guard, "fetch_run", return_value=RUN), \
             mock.patch.object(guard, "check_refs", side_effect=[None, guard.GuardError("changed")]), \
             mock.patch.object(guard.mirror_releases, "ci_ready", return_value=True):
            with self.assertRaises(guard.GuardError):
                guard.require_ci("486", SHA)

    def test_rejects_wrong_master_on_either_server(self):
        for source_sha, github_sha in (("b" * 40, SHA), (SHA, "b" * 40)):
            with self.subTest(source=source_sha, github=github_sha), \
                 mock.patch.object(guard.mirror_releases, "source", return_value={"commit": {"id": source_sha}}), \
                 mock.patch.object(guard.mirror_releases, "github", return_value={"commit": {"sha": github_sha}}):
                with self.assertRaises(guard.GuardError):
                    guard.check_refs(SHA)

    def test_rejects_index_flags_dirty_or_ignored_sources(self):
        for values in ([b"", b"", b""], [b"S source\n", b"", b""],
                       [b"H source\n", b" M source\n", b""],
                       [b"H source\n", b"", b"ignored.go\n"]):
            with self.subTest(values=values), mock.patch.object(guard, "git", side_effect=values):
                with self.assertRaises(guard.GuardError):
                    guard.check_clean_source()
        with mock.patch.object(guard, "git", side_effect=[b"H source\n", b"", b""]):
            guard.check_clean_source()


class ArtifactWorkflowTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.workflow = (Path(__file__).resolve().parents[2] / ".github/workflows/native-artifacts.yml").read_text()

    def test_manual_read_only_workflow_requires_the_gate(self):
        workflow = self.workflow
        trigger = workflow.split("\npermissions:\n", 1)[0]
        self.assertIn("\n  workflow_dispatch:\n", trigger)
        self.assertNotIn("\n  push:", trigger)
        self.assertNotIn("\n  schedule:", trigger)
        self.assertIn("\n  contents: read\n  actions: read\n", workflow)
        self.assertNotIn("contents: write", workflow)
        self.assertIn("native_artifacts_guard.py --require-ci", workflow)
        self.assertEqual(workflow.count("native_artifacts_guard.py --source-only"), 4)
        self.assertIn("\n    needs: verify\n", workflow)
        for runner in ("ubuntu-24.04", "windows-2025", "macos-15", "macos-15-intel"):
            self.assertIn("runner: " + runner, workflow)
        for command in ("github-windows-package.sh", "github-macos-package.sh", "appimage.sh", "hdiutil verify"):
            self.assertIn(command, workflow)
        self.assertNotIn("gh release", workflow)
        self.assertNotIn("git push", workflow)
        self.assertNotIn("stable-channel.py", workflow)

    def test_manual_inputs_are_not_interpolated_into_shell(self):
        self.assertIn("AEGO_CI_PACKAGE_VERSION: ${{ inputs.version }}", self.workflow)
        self.assertIn("AEGO_CI_PACKAGE_COMMIT: ${{ inputs.commit }}", self.workflow)
        self.assertIn("AEGO_GITEA_RUN_ID: ${{ inputs.gitea_run }}", self.workflow)
        for expression in ("${{ inputs.version }}", "${{ inputs.commit }}", "${{ inputs.gitea_run }}"):
            self.assertEqual(self.workflow.count(expression), 1)


class MacOSDMGFixtureTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="aego-macos-dmg-fixture-")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.repo = self.root / "repo"
        self.bin = self.root / "bin"
        self.repo.mkdir()
        self.bin.mkdir()
        script = self.repo / "tools/ci/github-macos-package.sh"
        script.parent.mkdir(parents=True)
        script.write_text('''#!/bin/bash
set -eu
mkdir -p '.cache/github-ci/macos/Aego Editor.app' '.cache/github-ci/dist/macos'
printf fixture > '.cache/github-ci/dist/macos/Aego-Editor-macos.zip'
(cd .cache/github-ci/dist/macos && sha256sum Aego-Editor-macos.zip > SHA256SUMS.txt)
''')
        self.calls = self.root / "calls"
        self.stub("du", '''#!/bin/bash
set -eu
[[ "${MOCK_DU_FAIL:-0}" != 1 ]] || exit 23
printf '%s\\t%s\\n' "${MOCK_DU:-66944}" "$2"
''')
        self.stub("hdiutil", '''#!/bin/bash
set -eu
printf 'hdiutil %s\\n' "$*" >> "$MOCK_CALLS"
if [[ "$1" == create ]]; then
  [[ " $* " == *" -size "* && "${MOCK_CREATE_FAIL:-0}" != 1 ]] || exit 24
  for arg in "$@"; do last="$arg"; done
  printf dmg > "$last"
elif [[ "$1" == verify ]]; then
  [[ "${MOCK_VERIFY_FAIL:-0}" != 1 ]] || exit 25
  [[ -f "$2" ]] || exit 26
else
  exit 27
fi
''')
        self.stub("shasum", '''#!/bin/bash
set -eu
[[ "$1" == -a && "$2" == 256 ]] || exit 28
exec sha256sum "$3"
''')
        workflow = (Path(__file__).resolve().parents[2] / ".github/workflows/native-artifacts.yml").read_text()
        section = workflow.split("      - name: Package macOS app and DMG\n", 1)[1].split("\n      - ", 1)[0]
        self.shell = textwrap.dedent(section.split("        run: |\n", 1)[1])
        self.env = {"PATH": str(self.bin) + os.pathsep + os.defpath, "HOME": str(self.root),
                    "TMPDIR": str(self.root), "LC_ALL": "C", "MOCK_CALLS": str(self.calls),
                    "AEGO_CI_PACKAGE_VERSION": "0.12.5"}

    def stub(self, name, source):
        path = self.bin / name
        path.write_text(source)
        path.chmod(0o755)

    def run_package(self, shell=None, **overrides):
        return subprocess.run(["bash", "--noprofile", "--norc", "-e", "-o", "pipefail", "-c", shell or self.shell],
                              cwd=self.repo, env={**self.env, **overrides}, capture_output=True, text=True, timeout=20)

    def calls_text(self):
        return self.calls.read_text() if self.calls.exists() else ""

    def test_explicit_headroom_and_verification(self):
        result = self.run_package()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn(" -size 256m ", self.calls_text())
        self.assertIn("hdiutil verify ", self.calls_text())
        dist = self.repo / ".cache/github-ci/dist/macos"
        checked = subprocess.run(["sha256sum", "-c", "SHA256SUMS.txt"], cwd=dist, capture_output=True, text=True)
        self.assertEqual(checked.returncode, 0, checked.stdout + checked.stderr)
        before = (dist / "Aego-Editor-macos.dmg").read_bytes()
        self.assertNotEqual(self.run_package().returncode, 0)
        self.assertEqual((dist / "Aego-Editor-macos.dmg").read_bytes(), before)

    def test_larger_bundle_gets_more_than_minimum(self):
        result = self.run_package(MOCK_DU="204800")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn(" -size 328m ", self.calls_text())

    def test_invalid_or_failed_size_probes_are_rejected(self):
        for value in ("bad", "0", "1234567890"):
            with self.subTest(value=value):
                result = self.run_package(MOCK_DU=value)
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn("hdiutil create", self.calls_text())
        self.assertNotEqual(self.run_package(MOCK_DU_FAIL="1").returncode, 0)

    def test_create_and_verify_failures_are_not_ignored(self):
        self.assertNotEqual(self.run_package(MOCK_CREATE_FAIL="1").returncode, 0)
        self.assertNotIn("hdiutil verify", self.calls_text())
        self.assertNotEqual(self.run_package(MOCK_VERIFY_FAIL="1").returncode, 0)
        self.assertIn("hdiutil verify", self.calls_text())
        sums = self.repo / ".cache/github-ci/dist/macos/SHA256SUMS.txt"
        self.assertNotIn("Aego-Editor-macos.dmg", sums.read_text())

    def test_legacy_command_without_size_fails_control(self):
        legacy = self.shell.replace(' -size "${size_mb}m"', "")
        self.assertNotEqual(legacy, self.shell)
        self.assertNotEqual(self.run_package(shell=legacy).returncode, 0)


class LinuxPackageFixtureTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="aego-artifact-workflow-")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.repo = self.root / "repo"
        self.bin = self.root / "bin"
        self.bin.mkdir()
        (self.repo / "tools/release").mkdir(parents=True)
        (self.repo / "app/brand/png").mkdir(parents=True)
        (self.repo / "tools/release/README.txt").write_text("fixture readme\n")
        (self.repo / "app/brand/png/aego-256.png").write_bytes(b"fixture icon")
        self.stub("go", '''#!/bin/bash
set -eu
if [[ "$1" == env ]]; then
  case "$2" in GOOS) echo linux ;; GOARCH) echo amd64 ;; *) exit 21 ;; esac
elif [[ "$1" == build ]]; then
  while [[ $# -gt 0 && "$1" != -o ]]; do shift; done
  [[ $# -gt 1 ]] || exit 22
  cat > "$2" <<'END'
#!/bin/sh
printf 'aego %s (%s, fixture)\\n' "$AEGO_CI_PACKAGE_VERSION" "${MOCK_VERSION_SHA:-aaaaaaaaaaaa}"
END
  chmod +x "$2"
else
  exit 23
fi
''')
        self.stub("sh", '''#!/bin/bash
set -eu
[[ "$1" == tools/release/appimage.sh && -f "$2/VERSION" ]] || exit 24
for name in APPIMAGETOOL APPIMAGETOOL_SHA256 APPIMAGETOOL_URL APPIMAGE_RUNTIME APPIMAGE_RUNTIME_SHA256 APPIMAGE_RUNTIME_URL; do
  [[ ! -v "$name" ]] || exit 25
done
printf 'synthetic AppImage fixture\\n' > "$3"
''')
        marker = "      - name: Package Linux archive and AppImage\n"
        workflow = (Path(__file__).resolve().parents[2] / ".github/workflows/native-artifacts.yml").read_text()
        section = workflow.split(marker, 1)[1].split("\n      - ", 1)[0]
        self.shell = textwrap.dedent(section.split("        run: |\n", 1)[1])
        self.env = {
            "PATH": str(self.bin) + os.pathsep + os.defpath,
            "HOME": str(self.root), "TMPDIR": str(self.root), "LC_ALL": "C",
            "RUNNER_ARCH": "X64", "AEGO_CI_PACKAGE_VERSION": "0.12.5",
            "AEGO_CI_PACKAGE_COMMIT": SHA,
            "APPIMAGETOOL": "/untrusted/tool", "APPIMAGETOOL_URL": "https://invalid/",
            "APPIMAGETOOL_SHA256": "untrusted", "APPIMAGE_RUNTIME": "/untrusted/runtime",
            "APPIMAGE_RUNTIME_URL": "https://invalid/", "APPIMAGE_RUNTIME_SHA256": "untrusted",
        }

    def stub(self, name, source):
        path = self.bin / name
        path.write_text(source)
        path.chmod(0o755)

    def run_package(self, **overrides):
        return subprocess.run(["bash", "--noprofile", "--norc", "-e", "-o", "pipefail", "-c", self.shell],
                              cwd=self.repo, env={**self.env, **overrides}, capture_output=True, text=True, timeout=20)

    def test_creates_versioned_candidates_with_verified_checksums(self):
        result = self.run_package()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        root = self.repo / ".cache/native-artifacts-linux"
        dist = root / "dist"
        with tarfile.open(dist / "aego-0.12.5-linux-amd64.tar.gz") as archive:
            self.assertEqual(archive.extractfile("aego-0.12.5-linux-amd64/VERSION").read(), ("v0.12.5\n" + SHA + "\n").encode())
            self.assertIn("aego-0.12.5-linux-amd64/bin/aego", archive.getnames())
        self.assertTrue((dist / "Aego-0.12.5-x86_64.AppImage").is_file())
        self.assertIn("update_key=not injected", (dist / "CI-PROVENANCE.txt").read_text())
        checked = subprocess.run(["sha256sum", "-c", "SHA256SUMS.txt"], cwd=dist, capture_output=True, text=True)
        self.assertEqual(checked.returncode, 0, checked.stdout + checked.stderr)
        before = (dist / "SHA256SUMS.txt").read_bytes()
        self.assertNotEqual(self.run_package().returncode, 0)
        self.assertEqual((dist / "SHA256SUMS.txt").read_bytes(), before)

    def test_refuses_wrong_architecture_or_binary_commit(self):
        self.assertNotEqual(self.run_package(RUNNER_ARCH="ARM64").returncode, 0)
        self.assertFalse((self.repo / ".cache/native-artifacts-linux").exists())
        self.assertNotEqual(self.run_package(MOCK_VERSION_SHA="bbbbbbbbbbbb").returncode, 0)
        self.assertFalse((self.repo / ".cache/native-artifacts-linux/dist/SHA256SUMS.txt").exists())


if __name__ == "__main__":
    unittest.main()
