//go:build !windows

package release

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Run the real publication script in a temporary tree with fake builders and
// a fake API. No release, tag, installer or network service is touched.
func TestStableChannelRequiresEveryReleaseAsset(t *testing.T) {
	for _, tc := range []struct {
		name                                                                      string
		failInstaller, failPublication, existingDraft, failSignature, failChannel bool
	}{
		{name: "all uploads succeed"},
		{name: "installer upload fails", failInstaller: true},
		{name: "resume existing draft", existingDraft: true},
		{name: "publication fails", failPublication: true},
		{name: "signature verification fails", failSignature: true},
		{name: "stable promotion fails", failChannel: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			write := func(name, content string, mode os.FileMode) {
				t.Helper()
				p := filepath.Join(root, name)
				if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(content), mode); err != nil {
					t.Fatal(err)
				}
			}
			script, err := os.ReadFile("release.sh")
			if err != nil {
				t.Fatal(err)
			}
			write("tools/release/release.sh", string(script), 0755)
			write("tools/release/stable-channel.py", `import os, sys
verify = '--verify-only' in sys.argv
with open(os.environ['MOCK_LOG'], 'a') as log:
    log.write('CALL manifest-verification\n' if verify else 'CALL stable-promotion\n')
if verify and os.environ['MOCK_FAIL_SIGNATURE'] == '1': sys.exit(1)
if not verify and os.environ['MOCK_FAIL_CHANNEL'] == '1': sys.exit(1)
`, 0644)
			write("tools/release/README.txt", "fixture\n", 0644)
			write("tools/bootstrap-native.sh", "#!/bin/sh\nexit 0\n", 0755)
			write("engine/config.go", "var Version = \"0.12.0\"\n", 0644)
			write("app/brand/png/aego-256.png", "fixture", 0644)
			if err := os.MkdirAll(filepath.Join(root, "cmd/aego"), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(root, "cmd/aego-editor"), 0755); err != nil {
				t.Fatal(err)
			}
			bin := filepath.Join(root, "bin")
			if err := os.Mkdir(bin, 0755); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"basename", "bash", "cat", "chmod", "cp", "dirname", "gzip", "head", "mkdir", "python3", "rm", "rmdir", "sed", "sha256sum", "tar", "tr", "zip"} {
				p, err := exec.LookPath(name)
				if err != nil {
					t.Skipf("test tool %s unavailable: %v", name, err)
				}
				if err := os.Symlink(p, filepath.Join(bin, name)); err != nil {
					t.Fatal(err)
				}
			}
			write("bin/git", "#!/bin/sh\nprintf '0123456789abcdef\\n'\n", 0755)
			write("bin/sleep", "#!/bin/sh\nexit 0\n", 0755)
			write("bin/x86_64-w64-mingw32-gcc", "#!/bin/sh\nexit 0\n", 0755)
			write("bin/makensis", `#!/bin/sh
for arg do
  case "$arg" in -DOUT=*) file=${arg#-DOUT=} ;; esac
done
[ -n "$file" ] || exit 1
printf 'fake installer\n' > "$file"
`, 0755)
			write("bin/go", `#!/bin/sh
if [ "$1" = run ]; then exit 0; fi
[ "$1" = build ] || exit 1
while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ]; then file=$2; break; fi
  shift
done
[ -n "$file" ] || exit 1
cat > "$file" <<'EOF'
#!/bin/sh
while [ "$#" -gt 0 ]; do
  if [ "$1" = --out ]; then out=$2; break; fi
  shift
done
[ -n "$out" ] || exit 1
printf 'manifest\n' > "$out/release.json"
printf 'signature\n' > "$out/release.json.sig"
EOF
chmod +x "$file"
`, 0755)
			write("bin/curl", `#!/bin/sh
printf '%s\n' "$*" >> "$MOCK_LOG"
case "$*" in
  *'-w %{http_code}'*)
    case "$*" in
      *-setup.exe*) if [ "$MOCK_FAIL_INSTALLER" = 1 ]; then printf 500; exit 0; fi ;;
    esac
    printf 201 ;;
  *'/releases/tags/v0.12.0'*)
    if [ "$MOCK_EXISTING_DRAFT" = 1 ]; then printf '{"id":1,"tag_name":"v0.12.0","draft":true}'; fi ;;
  *'/releases/tags/stable'*) : ;;
  *'"tag_name":"v0.12.0"'*) printf '{"id":1,"tag_name":"v0.12.0","draft":true}' ;;
  *'"tag_name":"stable"'*) printf '{"id":2,"tag_name":"stable"}' ;;
  *'-X PATCH'*'"draft":false'*) if [ "$MOCK_FAIL_PUBLICATION" = 1 ]; then exit 1; else printf '{}'; fi ;;
  *'/assets'*) printf '[]' ;;
  *) printf '{}' ;;
esac
`, 0755)
			log := filepath.Join(root, "api.log")
			flag := func(b bool) string {
				if b {
					return "1"
				}
				return "0"
			}
			cmd := exec.Command("/bin/sh", filepath.Join(root, "tools/release/release.sh"), "v0.12.0")
			cmd.Dir = root
			cmd.Env = []string{"HOME=" + root, "PATH=" + bin, "MOCK_LOG=" + log, "MOCK_FAIL_INSTALLER=" + flag(tc.failInstaller),
				"MOCK_EXISTING_DRAFT=" + flag(tc.existingDraft), "MOCK_FAIL_PUBLICATION=" + flag(tc.failPublication),
				"MOCK_FAIL_SIGNATURE=" + flag(tc.failSignature), "MOCK_FAIL_CHANNEL=" + flag(tc.failChannel),
				"SKIP_APPIMAGE=1", "REQUIRE_WINDOWS=1", "AEGO_RELEASE_KEY=fixture", "AEGO_UPDATE_KEY=fixture",
				"GITEA_TOKEN=fixture", "GITEA_API=http://release-test.invalid/api/v1/repos/test/aego"}
			output, err := cmd.CombinedOutput()
			if (err != nil) != (tc.failInstaller || tc.failPublication || tc.failSignature || tc.failChannel) {
				t.Fatalf("release exit: %v\n%s", err, output)
			}
			calls, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			api := string(calls)
			if !strings.Contains(api, "CALL manifest-verification") {
				t.Fatalf("signature verification was not attempted:\n%s", api)
			}
			if tc.failSignature {
				if strings.Contains(api, "/releases") || !strings.Contains(string(output), "did not verify") {
					t.Fatalf("an invalid signature reached the API:\n%s\n%s", api, output)
				}
				return
			}
			if !strings.Contains(api, "-setup.exe") {
				t.Fatal("installer upload was not exercised")
			}
			if !tc.existingDraft && !strings.Contains(api, `"draft":true`) {
				t.Fatalf("the new release was not created as a draft:\n%s", api)
			}
			published := strings.Contains(api, `"draft":false`)
			if published == tc.failInstaller {
				t.Fatalf("version publication attempted=%v with failed installer=%v\n%s", published, tc.failInstaller, api)
			}
			publishedAt := strings.Index(api, `"draft":false`)
			if published && publishedAt < strings.LastIndex(api, "-w %{http_code}") {
				t.Fatalf("the version was published before all asset uploads completed:\n%s", api)
			}
			stableTouched := strings.Contains(api, "CALL stable-promotion")
			if stableTouched == (tc.failInstaller || tc.failPublication) {
				t.Fatalf("stable channel touched=%v after failure=%v\n%s", stableTouched, tc.failInstaller || tc.failPublication, api)
			}
			if stableTouched && strings.Index(api, "CALL stable-promotion") < publishedAt {
				t.Fatalf("stable channel was switched before the version was published:\n%s", api)
			}
			if tc.failInstaller && !strings.Contains(string(output), "v0.12.0 is incomplete or not fully published") {
				t.Fatalf("missing upload failure report:\n%s", output)
			}
			if tc.failChannel && !strings.Contains(string(output), "stable/channel") {
				t.Fatalf("missing stable channel failure report:\n%s", output)
			}
		})
	}
}
