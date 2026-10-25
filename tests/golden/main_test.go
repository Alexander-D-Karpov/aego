//go:build aego_golden

package golden

import (
	"os"
	"runtime"
	"testing"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "aego-config-")
	if err != nil {
		panic(err)
	}
	switch runtime.GOOS {
	case "darwin":
		os.Setenv("HOME", dir)
	case "windows":
		os.Setenv("AppData", dir)
	default:
		os.Setenv("XDG_CONFIG_HOME", dir)
	}
	os.Setenv("AEGO_LANG", "en")
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
