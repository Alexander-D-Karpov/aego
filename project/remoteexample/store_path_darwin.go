package remoteexample

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// macOS installs /var and /tmp as system-managed aliases for /private. Only
// these exact expected aliases are normalized; all user-controlled links in
// the resulting path are still rejected by noLinks.
func platformStorePath(abs string) (string, error) {
	for _, pair := range [][2]string{{"/var", "/private/var"}, {"/tmp", "/private/tmp"}} {
		from, to := pair[0], pair[1]
		if abs != from && !strings.HasPrefix(abs, from+string(filepath.Separator)) {
			continue
		}
		info, err := os.Lstat(from)
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return abs, nil
		}
		target, err := os.Readlink(from)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(from), target)
		}
		if filepath.Clean(target) != to {
			return "", fmt.Errorf("unexpected macOS system path alias %s", from)
		}
		suffix := strings.TrimPrefix(strings.TrimPrefix(abs, from), string(filepath.Separator))
		return filepath.Join(to, suffix), nil
	}
	return abs, nil
}
