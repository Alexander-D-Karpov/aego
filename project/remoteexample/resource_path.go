package remoteexample

import (
	"fmt"
	"path"
	"strings"
)

// validateResourcePath is the shared data-only path policy for both published
// manifests and locally built example packages. Keep it separate from cleanPath:
// the store also uses that lower-level portable-path check for copy IDs.
func validateResourcePath(name string) error {
	if err := cleanPath(name); err != nil {
		return err
	}
	if !allowedFile(name) || name == receiptName {
		return fmt.Errorf("not an allowed project resource: %s", name)
	}
	return nil
}

// The transport profile deliberately permits only portable ASCII relative
// paths. This prevents Windows device names, alternate data streams, case-only
// collisions and Unicode-normalization aliases across supported desktops.
func cleanPath(name string) error {
	if len(name) == 0 || len(name) > 240 || name != path.Clean(name) || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\:\x00<>\"|?*") {
		return fmt.Errorf("unsafe package path %q", name)
	}
	for _, c := range name {
		if c < 32 || c > 126 {
			return fmt.Errorf("non-portable package path %q", name)
		}
	}
	for _, part := range strings.Split(name, "/") {
		low := strings.ToLower(part)
		if part == "." || part == ".." || part == "" || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") || strings.HasPrefix(part, " ") {
			return fmt.Errorf("unsafe path component %q", part)
		}
		base := strings.SplitN(low, ".", 2)[0]
		if base == "con" || base == "prn" || base == "aux" || base == "nul" || base == "clock$" || (len(base) == 4 && (strings.HasPrefix(base, "com") || strings.HasPrefix(base, "lpt")) && base[3] >= '0' && base[3] <= '9') {
			return fmt.Errorf("reserved path component %q", part)
		}
		if low == ".git" || low == ".hg" || low == ".svn" || low == ".aego" || low == "node_modules" || low == ".env" || strings.HasPrefix(low, ".env.") {
			return fmt.Errorf("forbidden package directory/file %q", part)
		}
	}
	return nil
}

func allowedFile(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".aego", ".aetext", ".aeproj", ".aeproject", ".aescene", ".aeprefab", ".aes", ".aeanim", ".aeanimator", ".aemat", ".aeshader", ".aeshaderinc", ".aepreset", ".aetry", ".aeart", ".aemixer", ".png", ".jpg", ".jpeg", ".webp", ".ogg", ".wav", ".mp3", ".flac", ".ttf", ".otf", ".json", ".meta", ".txt", ".md", ".atlas", ".ase", ".aseprite", ".glsl", ".frag", ".vert", ".lut", ".cube", ".csv":
		return true
	}
	switch strings.ToLower(path.Base(name)) {
	case "license", "copying", "notice":
		return true
	}
	return false
}
