//go:build linux

package release

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestStableChannelPromotionFailures(t *testing.T) {
	for _, name := range []string{"python3", "openssl"} {
		if _, err := exec.LookPath(name); err != nil {
			t.Fatalf("%s is required to verify release signatures: %v", name, err)
		}
	}
	cmd := exec.Command("python3", "test_stable_channel.py")
	for _, item := range os.Environ() {
		if !strings.HasPrefix(item, "AEGO_STAGE_SCRIPT=") && !strings.HasPrefix(item, "TMPDIR=") && !strings.HasPrefix(item, "PYTHONDONTWRITEBYTECODE=") {
			cmd.Env = append(cmd.Env, item)
		}
	}
	cmd.Env = append(cmd.Env, "TMPDIR="+t.TempDir(), "PYTHONDONTWRITEBYTECODE=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("stable channel tests: %v\n%s", err, output)
	}
}
