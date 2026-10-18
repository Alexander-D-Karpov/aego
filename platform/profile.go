package platform

import (
	"os"
	"path/filepath"

	"aego/core/errs"
	"aego/core/log"
)

const (
	DefaultProfile = "default"
	ProfileFile    = "settings.aetext"
)

func ValidProfile(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

func ProfileDir(module, profile string) (string, error) {
	if profile == "" {
		profile = DefaultProfile
	}
	if !ValidProfile(profile) {
		return "", errs.New(errs.PlatformConfig, "profile name may use letters, digits, - and _", log.F("profile", profile))
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", errs.Wrap(err, errs.PlatformConfig, "there is no user config directory")
	}
	return filepath.Join(base, "aego", filepath.FromSlash(module), "profiles", profile), nil
}

func ProfilePath(module, profile string) (string, error) {
	dir, err := ProfileDir(module, profile)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ProfileFile), nil
}

func ReadProfile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return data, err
}

func WriteProfile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".settings-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}
