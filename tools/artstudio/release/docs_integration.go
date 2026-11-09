package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// integrateDocs installs only the guide HTML and its three small host assets.
// The editor, examples and WASM remain in the single pinned release artifact.
// Existing different files are never replaced, and all destinations are rooted
// in the caller's chosen site directory.
func integrateDocs(dir, site, expected string) (map[string]any, error) {
	m, hash, err := verify(dir, expected)
	if err != nil {
		return nil, err
	}
	files := make(map[string]resource, len(m.Files))
	for _, file := range m.Files {
		files[file.Path] = file
	}
	artifact, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer artifact.Close()
	readVerified := func(name string) ([]byte, error) {
		want, ok := files[name]
		if !ok {
			return nil, fmt.Errorf("resource missing from manifest: %s", name)
		}
		st, err := artifact.Lstat(name)
		if err != nil || !st.Mode().IsRegular() || st.Size() != want.Bytes {
			return nil, fmt.Errorf("invalid artifact resource: %s", name)
		}
		f, err := artifact.Open(name)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		b, err := io.ReadAll(io.LimitReader(f, want.Bytes+1))
		if err != nil || int64(len(b)) != want.Bytes || digest(b) != want.SHA256 {
			return nil, fmt.Errorf("artifact resource changed: %s", name)
		}
		return b, nil
	}
	data, err := readVerified("docs/integration.json")
	if err != nil {
		return nil, err
	}
	var spec struct {
		BuildID, EditorURL, EmbedScript, ProtocolScript string
		Protocol                                        int
	}
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, err
	}
	editor := m.AppOrigin + "/r/" + m.BuildID + "/"
	if spec.BuildID != m.BuildID || spec.Protocol != m.ProtocolVersion || spec.EditorURL != editor {
		return nil, fmt.Errorf("documentation metadata does not match manifest")
	}
	if !strings.HasPrefix(spec.EmbedScript, "assets/docs-embed.") || !strings.HasSuffix(spec.EmbedScript, ".js") || !strings.HasPrefix(spec.ProtocolScript, "assets/protocol.") || !strings.HasSuffix(spec.ProtocolScript, ".js") {
		return nil, fmt.Errorf("documentation metadata names unexpected host scripts")
	}
	var style string
	for _, file := range m.Files {
		if strings.HasPrefix(file.Path, "assets/docs.") && strings.HasSuffix(file.Path, ".css") && hashedFile.MatchString(file.Path) {
			if style != "" {
				return nil, fmt.Errorf("ambiguous guide stylesheet")
			}
			style = file.Path
		}
	}
	for _, file := range []string{spec.EmbedScript, spec.ProtocolScript, style} {
		if _, ok := files[file]; !ok || !strings.HasPrefix(file, "assets/") || !hashedFile.MatchString(file) {
			return nil, fmt.Errorf("missing or unversioned guide asset %q", file)
		}
	}
	if len(m.Examples) == 0 {
		return nil, fmt.Errorf("artifact has no lesson examples")
	}
	items := map[string][]byte{}
	for _, file := range []string{spec.EmbedScript, spec.ProtocolScript, style} {
		b, err := readVerified(file)
		if err != nil {
			return nil, err
		}
		items[file] = b
	}
	for _, example := range m.Examples {
		for _, language := range []string{"en", "ru"} {
			rel := path.Join("docs", language, example.ID, "index.html")
			if _, ok := files[rel]; !ok {
				return nil, fmt.Errorf("guide missing from manifest: %s", rel)
			}
			b, err := readVerified(rel)
			if err != nil {
				return nil, err
			}
			for _, marker := range []string{`data-editor="` + html.EscapeString(editor) + `"`, `data-build="` + html.EscapeString(m.BuildID) + `"`, "../../../" + style, "../../../" + spec.EmbedScript, "../../../" + spec.ProtocolScript} {
				if !bytes.Contains(b, []byte(marker)) {
					return nil, fmt.Errorf("guide %s does not reference pinned integration: %s", rel, marker)
				}
			}
			items[path.Join("asset-studio", language, example.ID, "index.html")] = b
		}
	}
	manuals := 0
	for _, file := range m.Files {
		if (strings.HasPrefix(file.Path, "docs/en/guide/") || strings.HasPrefix(file.Path, "docs/ru/guide/")) && strings.HasSuffix(file.Path, "/index.html") {
			b, err := readVerified(file.Path)
			if err != nil {
				return nil, err
			}
			if !bytes.Contains(b, []byte("../../../../"+style)) {
				return nil, fmt.Errorf("manual missing verified style: %s", file.Path)
			}
			items["asset-studio/"+strings.TrimPrefix(file.Path, "docs/")] = b
			manuals++
		}
	}
	items["asset-studio/integration.json"] = jsonBytes(map[string]any{"buildID": m.BuildID, "editorURL": editor, "protocol": m.ProtocolVersion, "manifestSHA256": hash})
	if err := noSymlinkPath(site); err != nil {
		return nil, err
	}
	artifactAbs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	siteAbs, err := filepath.Abs(site)
	if err != nil {
		return nil, err
	}
	if siteAbs == artifactAbs || strings.HasPrefix(siteAbs, artifactAbs+string(filepath.Separator)) {
		return nil, fmt.Errorf("documentation output cannot be inside the immutable artifact")
	}
	root, err := os.OpenRoot(site)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	// Check the complete destination set before writing any file. An identical
	// prior result may be reused, but a conflicting file must be kept untouched.
	for rel, want := range items {
		parts := strings.Split(rel, "/")
		for i := 1; i < len(parts); i++ {
			parent := path.Join(parts[:i]...)
			st, err := root.Lstat(parent)
			if err != nil && !os.IsNotExist(err) {
				return nil, err
			}
			if err == nil && (!st.IsDir() || st.Mode()&os.ModeSymlink != 0) {
				return nil, fmt.Errorf("unsafe site parent: %s", parent)
			}
		}
		st, err := root.Lstat(rel)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !st.Mode().IsRegular() || st.Size() > maxFileBytes {
			return nil, fmt.Errorf("site destination is not a regular file: %s", rel)
		}
		old, err := root.ReadFile(rel)
		if err != nil || !bytes.Equal(old, want) {
			return nil, fmt.Errorf("site destination already differs: %s", rel)
		}
	}
	for rel, body := range items {
		if st, err := root.Lstat(rel); err == nil {
			if !st.Mode().IsRegular() || st.Size() > maxFileBytes {
				return nil, fmt.Errorf("site destination changed during integration: %s", rel)
			}
			old, readErr := root.ReadFile(rel)
			if readErr != nil || !bytes.Equal(old, body) {
				return nil, fmt.Errorf("site destination changed during integration: %s", rel)
			}
			continue
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		if err := root.MkdirAll(path.Dir(rel), 0o755); err != nil {
			return nil, err
		}
		f, err := root.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return nil, err
		}
		_, writeErr := f.Write(body)
		closeErr := f.Close()
		if writeErr != nil {
			return nil, writeErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
	}
	return map[string]any{"status": "PASS", "buildID": m.BuildID, "editorURL": editor, "manifestSHA256": hash, "guides": len(m.Examples) * 2, "manuals": manuals}, nil
}
