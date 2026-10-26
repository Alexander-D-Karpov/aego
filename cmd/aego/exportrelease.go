package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aego/app/export"
	"aego/project"
	upd "aego/update"
)

// exportRelease packs an exported game into one archive and writes the signed release.json its copies update from (D362).
func exportRelease(p *project.Project, rep *export.Report, channel, base, keyFile string) error {
	goos, arch, ok := strings.Cut(rep.Target, "/")
	if !ok {
		return errors.New("--release-json makes releases of desktop exports; a web export updates by itself")
	}
	if base == "" {
		url := ""
		for _, b := range p.Extra {
			if b.Head == "update" {
				if v, ok := b.Field("url"); ok {
					url = v.Str
				}
			}
		}
		if url == "" {
			return errors.New("project.aego has no update { url } and no --base-url was given")
		}
		base = strings.TrimSuffix(url, "/") + "/" + string(upd.ParseChannel(channel))
	}
	key := os.Getenv("AEGO_RELEASE_KEY")
	if keyFile == "" {
		keyFile = filepath.Join(prefsDir(), "keys", "release.key")
	}
	if key == "" {
		b, err := os.ReadFile(keyFile)
		if err != nil {
			return fmt.Errorf("no signing key: %w; make one with aego release keygen", err)
		}
		key = string(b)
	}
	version := p.GameVersion
	if version == "" {
		version = "0.0.0"
	}
	name := fmt.Sprintf("%s-%s-%s-%s", filepath.Base(p.Module), version, goos, arch)
	dir := filepath.Dir(rep.Out)
	var file string
	var err error
	if goos == "windows" {
		file = filepath.Join(dir, name+".zip")
		err = zipDir(rep.Out, filepath.Base(p.Module), file)
	} else {
		file = filepath.Join(dir, name+".tar.gz")
		err = tarDir(rep.Out, filepath.Base(p.Module), file)
	}
	if err != nil {
		return err
	}
	sum, size, err := upd.Sum(file)
	if err != nil {
		return err
	}
	kind := upd.KindTarball
	if goos == "windows" {
		kind = upd.KindZip
	}
	r := &upd.Release{Version: version, Channel: upd.ParseChannel(channel), Published: time.Now().UTC().Truncate(time.Second),
		Assets: []upd.Asset{{OS: goos, Arch: arch, Kind: kind, URL: strings.TrimSuffix(base, "/") + "/" + filepath.Base(file), Size: size, SHA256: sum}}}
	data, err := upd.Encode(r)
	if err != nil {
		return err
	}
	sig, err := upd.Sign(data, key)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, upd.ReleaseFile), data, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, upd.SigFile), sig, 0o644); err != nil {
		return err
	}
	fmt.Printf("release: %s, %s and %s; upload them to %s\n", filepath.Base(file), upd.ReleaseFile, upd.SigFile, base)
	return nil
}

// walk calls fn for every file under dir with its path inside the archive's one folder.
func walk(dir, top string, fn func(path, name string, info os.FileInfo) error) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		return fn(path, filepath.ToSlash(filepath.Join(top, rel)), info)
	})
}

func tarDir(dir, top, out string) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	err = walk(dir, top, func(path, name string, info os.FileInfo) error {
		h, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		h.Name = name
		if info.IsDir() {
			h.Name += "/"
		}
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(tw, in)
		return err
	})
	if err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func zipDir(dir, top, out string) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	err = walk(dir, top, func(path, name string, info os.FileInfo) error {
		if info.IsDir() {
			return nil
		}
		h, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		h.Name, h.Method = name, zip.Deflate
		w, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(w, in)
		return err
	})
	if err != nil {
		return err
	}
	return zw.Close()
}
