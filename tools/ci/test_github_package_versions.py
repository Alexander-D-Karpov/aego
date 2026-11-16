"""Exercise package scripts with disposable files and stub platform tools.

These tests do not build a binary or establish Windows/macOS compatibility.
"""
import base64
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
SCRIPTS = {
    'macos': ROOT / 'tools/ci/github-macos-package.sh',
    'windows': ROOT / 'tools/ci/github-windows-package.sh',
}
SHA = 'a' * 40
PUBLIC = base64.b64encode(b'a' * 32).decode('ascii')
FINGERPRINT = '06096a4da6dac0b1ee5a8aecc665700bb6a98066f2dffdb30e4d3096877da59e'


class PackageVersionTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix='aego-package-version-')
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.repo = self.root / 'repo'
        self.bin = self.root / 'bin'
        (self.repo / 'engine').mkdir(parents=True)
        (self.repo / 'tools/release').mkdir(parents=True)
        self.bin.mkdir()
        (self.repo / 'engine/config.go').write_text('package engine\nvar Version = "0.12.5"\n')
        (self.repo / 'tools/release/README.txt').write_text('fixture\n')
        self.calls = self.root / 'calls'
        self.env = {
            'PATH': str(self.bin) + os.pathsep + os.defpath,
            'HOME': str(self.root),
            'TMPDIR': str(self.root),
            'LC_ALL': 'C',
            'MOCK_ROOT': str(self.repo),
            'MOCK_CALLS': str(self.calls),
            'MOCK_COMMIT': SHA,
            'MOCK_FLAGS': 'H engine/config.go\n',
            'MOCK_GOOS': 'darwin',
            'MOCK_GOARCH': 'arm64',
            'MOCK_PLIST_VERSION': '0.12.5',
            'MOCK_UPDATE_KEY': PUBLIC,
            'MOCK_UPDATE_FINGERPRINT': FINGERPRINT,
        }
        self.stub('git', '''#!/bin/bash
set -eu
case "$*" in
  'rev-parse HEAD') printf '%s\\n' "$MOCK_COMMIT" ;;
  'ls-files -v') printf '%s' "$MOCK_FLAGS" ;;
  'status --porcelain --untracked-files=all')
    [[ "${MOCK_STATUS_FAIL:-0}" != 1 ]] || exit 17
    printf '%s' "${MOCK_STATUS:-}" ;;
  'ls-files --others --ignored --exclude-standard -- . :(exclude).cache/**')
    [[ "${MOCK_IGNORED_FAIL:-0}" != 1 ]] || exit 18
    printf '%s' "${MOCK_IGNORED:-}" ;;
  *) printf 'unexpected git call: %s\\n' "$*" >&2; exit 19 ;;
esac
''')
        self.stub('go', '''#!/bin/bash
set -eu
printf 'go %s GOFLAGS=%s GOWORK=%s\\n' "$*" "${GOFLAGS:-}" "${GOWORK:-}" >> "$MOCK_CALLS"
if [[ "$1" == env ]]; then
  case "$2" in GOOS) printf '%s\\n' "$MOCK_GOOS" ;; GOARCH) printf '%s\\n' "$MOCK_GOARCH" ;; *) exit 20 ;; esac
elif [[ "$1" == build ]]; then
  while [[ $# -gt 0 && "$1" != -o ]]; do shift; done
  [[ $# -gt 1 ]] || exit 21
  mkdir -p "$(dirname "$2")"
  : > "$2"
elif [[ "$1" == run && "$2" == ./tools/appbundle ]]; then
  mkdir -p "$MOCK_ROOT/.cache/github-ci/macos/Aego Editor.app/Contents"
  : > "$MOCK_ROOT/.cache/github-ci/macos/Aego Editor.app/Contents/Info.plist"
fi
''')
        self.stub('python3', '''#!/bin/bash
set -eu
if [[ "${1:-}" == -B ]]; then shift; fi
[[ "${1:-}" == tools/ci/release_update_key.py ]] || exit 31
printf 'update-helper %s\n' "${2:-}" >> "$MOCK_CALLS"
case "${2:-}" in
  --value) [[ "${MOCK_KEY_FAIL:-0}" != 1 ]] || exit 32; printf '%s\n' "$MOCK_UPDATE_KEY" ;;
  --fingerprint) [[ "${AEGO_CI_UPDATE_PUBLIC_KEY:-}" == "$MOCK_UPDATE_KEY" ]] || exit 33; printf '%s\n' "$MOCK_UPDATE_FINGERPRINT" ;;
  --smoke|--check-binary)
    [[ "${AEGO_CI_UPDATE_PUBLIC_KEY:-}" == "$MOCK_UPDATE_KEY" && -f "${3:-}" && "${MOCK_VERIFY_FAIL:-0}" != 1 ]] || exit 34 ;;
  *) exit 35 ;;
esac
''')
        self.stub('py', '''#!/bin/bash
set -eu
[[ "${1:-}" == -3 ]] || exit 36
shift
exec python3 "$@"
''')
        self.stub('plutil', '''#!/bin/bash
set -eu
printf 'plutil %s\\n' "$*" >> "$MOCK_CALLS"
if [[ "$1" == -extract ]]; then printf '%s' "$MOCK_PLIST_VERSION"; fi
''')
        self.stub('codesign', '#!/bin/bash\nprintf "codesign %s\\n" "$*" >> "$MOCK_CALLS"\n')
        self.stub('otool', '#!/bin/bash\nprintf "/fake:\\n /System/Library/fake.dylib (compatibility version 1.0.0)\\n"\n')
        self.stub('ditto', '#!/bin/bash\nfor arg in "$@"; do last="$arg"; done\n: > "$last"\n')
        self.stub('shasum', '#!/bin/bash\n[[ "$1" == -a && "$2" == 256 ]] || exit 1\nexec sha256sum "$3"\n')
        self.stub('objdump', '#!/bin/bash\nprintf "fake PE\\n"\n')
        self.stub('cygpath', '''#!/bin/bash
set -eu
printf 'cygpath %s\\n' "$*" >> "$MOCK_CALLS"
if [[ -n "${MOCK_CYGPATH_FAILURE:-}" && "$2" == *"$MOCK_CYGPATH_FAILURE" ]]; then exit 17; fi
if [[ -n "${MOCK_CYGPATH_EMPTY:-}" && "$2" == *"$MOCK_CYGPATH_EMPTY" ]]; then exit 0; fi
printf '%s\\n' "$2"
''')
        self.stub('pwsh', '''#!/bin/bash
printf 'pwsh called\\npwsh-stage=<%s>\\npwsh-zip=<%s>\\n' "$AEGO_CI_STAGE" "$AEGO_CI_ZIP" >> "$MOCK_CALLS"
: > "$AEGO_CI_ZIP"
''')
        self.stub('makensis', '''#!/bin/bash
set -eu
printf 'makensis %s\\n' "$*" >> "$MOCK_CALLS"
printf 'makensis-arg=<%s>\\n' "$@" >> "$MOCK_CALLS"
for arg in "$@"; do
  case "$arg" in -DOUT=*) : > "${arg#-DOUT=}" ;; esac
done
''')

    def stub(self, name, source):
        path = self.bin / name
        path.write_text(source)
        path.chmod(0o755)

    def run_package(self, platform, *, versioned=True, overrides=None):
        env = self.env.copy()
        env.update({
            'MOCK_GOOS': 'darwin' if platform == 'macos' else 'windows',
            'MOCK_GOARCH': 'arm64' if platform == 'macos' else 'amd64',
            'RUNNER_OS': 'macOS' if platform == 'macos' else 'Windows',
            'RUNNER_ARCH': 'ARM64' if platform == 'macos' else 'X64',
        })
        if versioned:
            env.update({
                'AEGO_CI_PACKAGE_VERSION': '0.12.5',
                'AEGO_CI_PACKAGE_COMMIT': SHA,
                'GITHUB_SHA': SHA,
                'GITHUB_ACTIONS': 'true',
                'GITHUB_SERVER_URL': 'https://github.com',
                'GITHUB_EVENT_NAME': 'workflow_dispatch',
                'GITHUB_WORKFLOW': 'aego-native-artifacts-only',
                'RUNNER_ENVIRONMENT': 'github-hosted',
                'GOFLAGS': '-ldflags=untrusted',
                'GOWORK': '/untrusted/go.work',
            })
        for key, value in (overrides or {}).items():
            if value is None:
                env.pop(key, None)
            else:
                env[key] = value
        return subprocess.run(['bash', str(SCRIPTS[platform])], cwd=self.repo, env=env,
                              capture_output=True, text=True, timeout=10)

    def calls_text(self):
        return self.calls.read_text() if self.calls.exists() else ''

    def test_github_native_runs_package_guard_tests_on_linux(self):
        workflow = (ROOT / '.github/workflows/github-native.yml').read_text()
        self.assertIn(
            "      - name: Test package script guards\n"
            "        if: runner.os == 'Linux'\n"
            "        run: python3 -B -m unittest discover -s tools/ci -p test_github_package_versions.py\n",
            workflow,
        )

    def test_existing_ci_keeps_its_metadata_and_outputs(self):
        for platform in SCRIPTS:
            with self.subTest(platform=platform):
                self.calls.unlink(missing_ok=True)
                result = self.run_package(platform, versioned=False)
                self.assertEqual(result.returncode, 0, result.stderr)
                calls = self.calls_text()
                self.assertNotIn('update-helper', calls)
                if platform == 'macos':
                    self.assertIn('-version 0.0.0', calls)
                    self.assertNotIn('plutil -extract', calls)
                    self.assertFalse((self.repo / '.cache/github-ci/dist/macos/CI-PROVENANCE.txt').exists())
                else:
                    self.assertIn('-version 0.0.0', calls)
                    self.assertIn('-DVERSION=0.0.0', calls)
                    self.assertEqual((self.repo / '.cache/github-ci/windows-stage/VERSION').read_text(),
                                     'CI build\n' + SHA + '\n')
                    self.assertFalse((self.repo / '.cache/github-ci/dist/installer/CI-PROVENANCE.txt').exists())

    def test_verified_source_version_reaches_all_package_metadata(self):
        for platform in SCRIPTS:
            with self.subTest(platform=platform):
                self.calls.unlink(missing_ok=True)
                result = self.run_package(platform)
                self.assertEqual(result.returncode, 0, result.stderr)
                calls = self.calls_text()
                self.assertIn('-version 0.12.5', calls)
                self.assertIn('GOFLAGS= GOWORK=off', calls)
                self.assertIn('-X=aego/update.PublicKey=' + PUBLIC, calls)
                if platform == 'macos':
                    self.assertIn('plutil -extract CFBundleShortVersionString', calls)
                    self.assertIn('plutil -extract CFBundleVersion', calls)
                    paths = ['macos']
                    self.assertIn('not notarized', (self.repo / '.cache/github-ci/dist/macos/CI-PROVENANCE.txt').read_text())
                    self.assertIn('update-helper --check-binary', calls)
                else:
                    self.assertIn('-DVERSION=0.12.5', calls)
                    self.assertEqual((self.repo / '.cache/github-ci/windows-stage/VERSION').read_text(),
                                     '0.12.5\n' + SHA + '\n')
                    paths = ['portable', 'installer']
                    self.assertIn('update-helper --smoke', calls)
                    self.assertIn('update-helper --check-binary', calls)
                for path in paths:
                    data = (self.repo / '.cache/github-ci/dist' / path / 'CI-PROVENANCE.txt').read_text()
                    self.assertIn('version=0.12.5\ncommit=' + SHA + '\n', data)
                    self.assertIn('update_key=embedded\nupdate_key_sha256=' + FINGERPRINT + '\n', data)

    def test_versioned_packaging_rejects_bad_key_before_build(self):
        for platform in SCRIPTS:
            for override in ({'MOCK_KEY_FAIL': '1'}, {'MOCK_UPDATE_KEY': 'bad'}):
                with self.subTest(platform=platform, override=override):
                    self.calls.unlink(missing_ok=True)
                    result = self.run_package(platform, overrides=override)
                    self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
                    self.assertNotIn('go build ', self.calls_text())

    def test_versioned_packaging_stops_when_binary_key_verification_fails(self):
        for platform in SCRIPTS:
            with self.subTest(platform=platform):
                self.calls.unlink(missing_ok=True)
                result = self.run_package(platform, overrides={'MOCK_VERIFY_FAIL': '1'})
                self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
                calls = self.calls_text()
                self.assertIn('go build ', calls)
                self.assertNotIn('makensis ', calls)
                self.assertNotIn('pwsh called', calls)
                self.assertNotIn('go run ./tools/appbundle', calls)

    def test_macos_intel_target_is_accepted(self):
        result = self.run_package('macos', overrides={'RUNNER_ARCH': 'X64', 'MOCK_GOARCH': 'amd64'})
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_windows_accepts_a_crlf_checkout(self):
        (self.repo / 'engine/config.go').write_bytes(b'package engine\r\nvar Version = "0.12.5"\r\n')
        result = self.run_package('windows')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('-DVERSION=0.12.5', self.calls_text())

    def test_windows_passes_paths_with_spaces_as_single_arguments(self):
        repo = self.root / 'repo with spaces'
        self.repo.rename(repo)
        self.repo = repo
        self.env['MOCK_ROOT'] = str(repo)
        result = self.run_package('windows')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        root = repo / '.cache/github-ci'
        calls = self.calls_text()
        self.assertIn(f'pwsh-stage=<{root / "windows-stage"}>\n', calls)
        self.assertIn(f'pwsh-zip=<{root / "dist/portable/aego-windows-amd64-portable.zip"}>\n', calls)
        self.assertIn(f'makensis-arg=<-DSRC={root / "windows-stage"}>\n', calls)
        self.assertIn(f'makensis-arg=<-DOUT={root / "dist/installer/aego-windows-amd64-setup.exe"}>\n', calls)
        self.assertIn(f'makensis-arg=<-DICON={root / "aego.ico"}>\n', calls)
        self.assertIn(f'makensis-arg=<{repo / "tools/release/windows/aego.nsi"}>\n', calls)

    def test_windows_rejects_failed_or_empty_path_conversions(self):
        paths = ('/windows-stage', '/aego-windows-amd64-portable.zip',
                 '/aego-windows-amd64-setup.exe', '/aego.ico', '/aego.nsi')
        for versioned in (False, True):
            for failure in ('MOCK_CYGPATH_FAILURE', 'MOCK_CYGPATH_EMPTY'):
                for path in paths:
                    with self.subTest(versioned=versioned, failure=failure, path=path):
                        fixture = type(self)('runTest')
                        fixture.setUp()
                        try:
                            result = fixture.run_package('windows', versioned=versioned,
                                                         overrides={failure: path})
                            calls = fixture.calls_text()
                            self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr + calls)
                            self.assertNotIn('pwsh called', calls)
                            self.assertNotIn('makensis ', calls)
                        finally:
                            fixture.doCleanups()

    def test_versioned_packaging_rejects_unverified_inputs_before_build(self):
        bad = [
            {'AEGO_CI_PACKAGE_VERSION': None},
            {'AEGO_CI_PACKAGE_COMMIT': None},
            {'AEGO_CI_PACKAGE_VERSION': ''},
            {'AEGO_CI_PACKAGE_VERSION': '0.12.5-dev'},
            {'AEGO_CI_PACKAGE_VERSION': '00.12.5'},
            {'AEGO_CI_PACKAGE_VERSION': '65536.12.5'},
            {'AEGO_CI_PACKAGE_VERSION': '0.12.6'},
            {'AEGO_CI_PACKAGE_COMMIT': 'b' * 40},
            {'GITHUB_SHA': 'b' * 40},
            {'MOCK_COMMIT': 'b' * 40},
            {'GITHUB_EVENT_NAME': 'push'},
            {'GITHUB_WORKFLOW': 'github-native'},
            {'RUNNER_ENVIRONMENT': 'self-hosted'},
            {'MOCK_GOARCH': '386'},
            {'MOCK_FLAGS': 'S engine/config.go\n'},
            {'MOCK_FLAGS': 'h engine/config.go\n'},
            {'MOCK_STATUS': ' M engine/config.go\n'},
            {'MOCK_IGNORED': 'engine/res/docs/canary.key\n'},
            {'MOCK_STATUS_FAIL': '1'},
            {'MOCK_IGNORED_FAIL': '1'},
        ]
        for platform in SCRIPTS:
            for override in bad:
                with self.subTest(platform=platform, override=override):
                    self.calls.unlink(missing_ok=True)
                    result = self.run_package(platform, overrides=override)
                    self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
                    self.assertNotIn('go build ', self.calls_text())
                    stage = 'macos' if platform == 'macos' else 'windows-stage'
                    self.assertFalse((self.repo / '.cache/github-ci' / stage).exists())

    def test_macos_rejects_wrong_bundle_metadata(self):
        result = self.run_package('macos', overrides={'MOCK_PLIST_VERSION': '0.0.0'})
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Bundle version does not match', result.stderr)
        self.assertNotIn('codesign ', self.calls_text())

    def test_versioned_packaging_does_not_overwrite_existing_outputs(self):
        for platform in SCRIPTS:
            with self.subTest(platform=platform):
                path = self.repo / ('.cache/github-ci/dist/macos' if platform == 'macos'
                                    else '.cache/github-ci/aego.ico')
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text('existing')
                self.calls.unlink(missing_ok=True)
                result = self.run_package(platform)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn('Package output already exists', result.stderr)
                self.assertNotIn('go build ', self.calls_text())
                self.assertEqual(path.read_text(), 'existing')
                path.unlink()

    def test_versioned_packaging_rejects_symlinked_output_parent(self):
        dest = self.root / 'external-cache'
        dest.mkdir()
        (self.repo / '.cache').symlink_to(dest, target_is_directory=True)
        for platform in SCRIPTS:
            with self.subTest(platform=platform):
                result = self.run_package(platform)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn('Package output parent is a symlink', result.stderr)
                self.assertNotIn('go build ', self.calls_text())
                self.assertEqual(list(dest.iterdir()), [])

    def test_windows_does_not_overwrite_a_resource_object(self):
        path = self.repo / 'cmd/aego/rsrc_windows_amd64.syso'
        path.parent.mkdir(parents=True)
        path.write_text('existing')
        result = self.run_package('windows')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Package output already exists', result.stderr)
        self.assertNotIn('go build ', self.calls_text())
        self.assertEqual(path.read_text(), 'existing')

    def test_git_excludes_only_cache_from_ignored_source_check(self):
        # A disposable repository checks the real Git pathspec, without a commit.
        repo = self.root / 'git-fixture'
        repo.mkdir()
        git = str(Path(os.defpath.split(os.pathsep)[0]) / 'git')
        env = {'HOME': str(self.root), 'PATH': os.defpath, 'LC_ALL': 'C', 'GIT_CONFIG_NOSYSTEM': '1'}
        subprocess.run([git, 'init', '-q', str(repo)], env=env, check=True)
        (repo / '.gitignore').write_text('.cache/\n*.syso\n')
        (repo / '.cache/native').mkdir(parents=True)
        (repo / '.cache/native/ignored.syso').write_text('fixture')
        (repo / 'other').mkdir()
        (repo / 'other/ignored.syso').write_text('fixture')
        result = subprocess.run([git, 'ls-files', '--others', '--ignored', '--exclude-standard',
                                 '--', '.', ':(exclude).cache/**'], cwd=repo, env=env,
                                capture_output=True, text=True, check=True)
        self.assertEqual(result.stdout.splitlines(), ['other/ignored.syso'])


if __name__ == '__main__':
    unittest.main()
