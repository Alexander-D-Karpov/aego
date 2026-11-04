package agentbridge

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"aego/app/agent"
	"aego/editor/core/document"
	"aego/project"
)

// Only authoring inputs enter the temporary project. The isolated run imports
// its own artifacts; it cannot overwrite the editor's index or last-good data.
func (b *Bridge) playOverlay() map[string][]byte {
	out := make(map[string][]byte)
	for _, d := range b.ctx.Docs.Scripts() {
		if d.Dirty() {
			out[d.Path] = bytes.Clone(d.Code.Bytes())
		}
	}
	for _, d := range b.ctx.Docs.AssetDocuments() {
		for _, s := range []*document.ScriptDocument{d.Source, d.Metadata} {
			if s != nil && s.Dirty() {
				out[s.Path] = bytes.Clone(s.Code.Bytes())
			}
		}
	}
	if len(out) == 0 {
		for _, d := range b.ctx.Docs.Docs() {
			if d.IsPrefab() && d.Dirty() {
				out[d.Path] = b.ctx.Docs.Snapshot(d)
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	for _, d := range b.ctx.Docs.Docs() {
		if d.Dirty() {
			out[d.Path] = b.ctx.Docs.Snapshot(d)
		}
	}
	out[project.FileName] = b.Project().Encode()
	return out
}
func materializePlay(root, dst string, overlay map[string][]byte) error {
	src, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer src.Close()
	out, err := os.OpenRoot(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	total := int64(0)
	count := 0
	put := func(rel string, r io.Reader) error {
		if !fs.ValidPath(rel) {
			return errors.New("invalid play input path")
		}
		name := filepath.FromSlash(rel)
		if err := out.MkdirAll(filepath.Dir(name), 0700); err != nil {
			return err
		}
		if count++; count > 25000 {
			return errors.New("too many play inputs")
		}
		f, err := out.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		n, err := io.Copy(f, io.LimitReader(r, (512<<20)-total+1))
		e := f.Close()
		total += n
		if total > 512<<20 {
			return errors.New("isolated play inputs exceed 512 MiB")
		}
		if err != nil {
			return err
		}
		return e
	}
	for _, dir := range project.DefaultDirs {
		err = fs.WalkDir(src.FS(), dir, func(rel string, d fs.DirEntry, e error) error {
			if e != nil {
				if rel == dir && errors.Is(e, fs.ErrNotExist) {
					return nil
				}
				return e
			}
			if d.IsDir() {
				return nil
			}
			if _, replaced := overlay[rel]; replaced {
				return nil
			}
			if !d.Type().IsRegular() {
				return fmt.Errorf("unsupported linked or special play input: %s", rel)
			}
			f, err := src.Open(filepath.FromSlash(rel))
			if err != nil {
				return err
			}
			defer f.Close()
			return put(rel, f)
		})
		if err != nil {
			return err
		}
	}
	for rel, buf := range overlay {
		if rel != project.FileName {
			ok := false
			for _, dir := range project.DefaultDirs {
				if strings.HasPrefix(rel, dir+"/") {
					ok = true
					break
				}
			}
			if !ok {
				return fmt.Errorf("unsaved input is outside the project's source folders: %s", rel)
			}
		}
		if err := put(rel, bytes.NewReader(buf)); err != nil {
			return err
		}
	}
	return nil
}
func keepShots(root, tmp string, shots []agent.Shot) error {
	src, e := os.OpenRoot(tmp)
	if e != nil {
		return e
	}
	defer src.Close()
	out, e := os.OpenRoot(root)
	if e != nil {
		return e
	}
	defer out.Close()
	for i := range shots {
		s := &shots[i]
		if s.Path == "" {
			continue
		}
		if !fs.ValidPath(s.Path) || !strings.HasPrefix(s.Path, ".aego/cache/runs/") {
			return errors.New("invalid shot path")
		}
		f, e := src.Open(filepath.FromSlash(s.Path))
		if e != nil {
			return e
		}
		rel := filepath.Join(".aego", "cache", "runs", filepath.Base(tmp)+"-"+filepath.Base(s.Path))
		if e = out.MkdirAll(filepath.Dir(rel), 0700); e != nil {
			f.Close()
			return e
		}
		to, e := out.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			f.Close()
			return e
		}
		_, err := io.Copy(to, f)
		f.Close()
		closeErr := to.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		s.Path = filepath.ToSlash(rel)
	}
	return nil
}
