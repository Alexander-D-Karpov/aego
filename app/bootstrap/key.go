package bootstrap

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"aego/core/errs"
	"aego/project"
)

var skipDirs = map[string]bool{
	".git":         true,
	".cache":       true,
	".aego":        true,
	".idea":        true,
	".vscode":      true,
	"testdata":     true,
	"node_modules": true,
	"exports":      true,
}

func CacheKey(p *project.Project, t Target, engineVersion string) (Key, error) {
	h := sha256.New()
	writeString(h, engineVersion)
	writeString(h, SDKVersion)
	writeString(h, runtime.Version())
	writeString(h, "fullpaths")
	writeString(h, t.OS)
	writeString(h, t.Arch)
	for _, tag := range t.Tags {
		writeString(h, tag)
	}
	for _, name := range []string{"go.mod", "go.sum"} {
		if data, err := os.ReadFile(filepath.Join(p.Root, name)); err == nil {
			writeString(h, name)
			h.Write(data)
		}
	}
	if err := hashTree(h, p.NativeDir(), p.Root); err != nil {
		return "", err
	}
	replaces, err := LocalReplaces(p.Root)
	if err != nil {
		return "", err
	}
	for _, dir := range replaces {
		writeString(h, "replace:"+filepath.ToSlash(dir))
		if err := hashTree(h, dir, dir); err != nil {
			return "", err
		}
	}
	return Key(hex.EncodeToString(h.Sum(nil))[:16]), nil
}

func writeString(h hash.Hash, s string) {
	h.Write([]byte(s))
	h.Write([]byte{0})
}

func LocalReplaces(root string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errs.Wrap(err, errs.VFSIO, "cannot read go.mod")
	}
	var out []string
	seen := map[string]bool{}
	inBlock := false
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := sc.Text()
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		switch {
		case line == "":
			continue
		case inBlock && line == ")":
			inBlock = false
			continue
		case line == "replace (":
			inBlock = true
			continue
		case strings.HasPrefix(line, "replace "):
			line = strings.TrimSpace(strings.TrimPrefix(line, "replace "))
		case !inBlock:
			continue
		}
		_, target, ok := strings.Cut(line, "=>")
		if !ok {
			continue
		}
		fields := strings.Fields(target)
		if len(fields) == 0 {
			continue
		}
		path := fields[0]
		if !isLocalPath(path) {
			continue
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, filepath.FromSlash(path))
		}
		path = filepath.Clean(path)
		if !seen[path] {
			seen[path] = true
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out, nil
}

func isLocalPath(p string) bool {
	return strings.HasPrefix(p, "./") || strings.HasPrefix(p, "../") ||
		p == "." || p == ".." || filepath.IsAbs(p)
}

func hashTree(h hash.Hash, dir, rel string) error {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && path == dir {
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() {
			if path == dir {
				return nil
			}
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot walk build inputs")
	}
	sort.Strings(files)
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			return errs.Wrap(err, errs.VFSIO, "cannot read a build input")
		}
		r, err := filepath.Rel(rel, path)
		if err != nil {
			r = path
		}
		writeString(h, filepath.ToSlash(r))
		h.Write(data)
	}
	return nil
}
