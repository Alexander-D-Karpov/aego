// Package audiosidecar defines an in-memory, fail-closed audio edit metadata
// envelope. It does not write files or enable EDL in the importer or editor.
// Existing metadata parsers reject its version 2 marker when they read it.
// That marker does not protect against an already-open older process using a
// cached version-1 record or writing stale metadata. Integration must not
// persist this envelope until the importer, runtime, guarded save path and
// cached-reader conflict handling support it together.
package audiosidecar

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"aego/asset"
	"aego/audio/edit"
	"aego/format/aetext"
)

const (
	formatVersion    = 2
	editField        = "audioEdit"
	maxMetadataBytes = 16 << 20
	maxNesting       = 64
)

type parsed struct {
	root     *aetext.Node
	settings *aetext.Node
	meta     *asset.SourceMeta
	version  int64
	edit     audioedit.Document
	present  bool
}

// Decode validates an audio sidecar and returns its immutable edit document.
// An ordinary version 1 sidecar without the reserved field returns present=false.
// A malformed, unsupported, ambiguous or identity-mismatched envelope is an
// error, never a request to fall back to original audio. It does not verify the
// actual encoded source's digest, read files or publish any artifact.
func Decode(raw []byte, sourcePath string) (doc audioedit.Document, present bool, err error) {
	p, err := parse(raw, sourcePath)
	if err != nil {
		return audioedit.Document{}, false, err
	}
	return p.edit.Clone(), p.present, nil
}

// Encode produces a candidate only in memory. It preserves unknown metadata,
// refuses lossy updates, and marks edited metadata version 2 so parsers that
// only support version 1 reject it when reading. A cached older record is not
// protected by the marker. This function does not grant permission to save or
// guard any source. A caller must verify source bytes and perform a guarded
// transaction only after all import and runtime paths support the format.
func Encode(raw []byte, sourcePath string, doc audioedit.Document) ([]byte, error) {
	p, err := parse(raw, sourcePath)
	if err != nil {
		return nil, err
	}
	if err := validateDocument(doc, p.meta, sourcePath); err != nil {
		return nil, err
	}
	doc = doc.Clone()
	if p.present && p.edit.Source != doc.Source {
		return nil, errors.New("audio edit cannot silently replace its source identity")
	}
	data, err := doc.Encode()
	if err != nil {
		return nil, err
	}
	if p.settings == nil {
		p.settings = &aetext.Node{Head: asset.SourceSettingsBlock}
		p.root.Blocks = append(p.root.Blocks, p.settings)
	}
	p.settings.SetField(editField, aetext.String(string(data)))
	setVersion(p.root, formatVersion)
	if _, err := writeSize(p.root, maxMetadataBytes); err != nil {
		return nil, err
	}
	out := aetext.Write(p.root)
	if len(out) > maxMetadataBytes {
		return nil, errors.New("encoded audio sidecar exceeds metadata budget")
	}
	return out, nil
}

func parse(raw []byte, sourcePath string) (*parsed, error) {
	if err := validSourcePath(sourcePath); err != nil {
		return nil, err
	}
	if len(raw) > maxMetadataBytes || !utf8.Valid(raw) {
		return nil, errors.New("audio sidecar is oversized or not UTF-8")
	}
	// aetext's parser is recursive. Check its nesting budget before parsing
	// untrusted metadata, while ignoring brackets inside quoted strings.
	if err := boundedSyntax(raw); err != nil {
		return nil, err
	}
	root, err := aetext.Parse(raw, sourcePath+asset.MetaSuffix)
	if err != nil {
		return nil, err
	}
	if root.Head != "meta" || root.Label != "" {
		return nil, errors.New("audio sidecar must be a metadata object")
	}
	p := &parsed{root: root, version: 1}
	versionCount := 0
	for _, field := range root.Attrs {
		if field.Name == "version" {
			versionCount++
			if field.Value.Kind != aetext.VInt || (field.Value.Int != 1 && field.Value.Int != formatVersion) {
				return nil, errors.New("unsupported audio metadata version")
			}
			p.version = field.Value.Int
		}
	}
	if versionCount > 1 {
		return nil, errors.New("ambiguous audio metadata version")
	}
	critical := make(map[string]bool)
	for _, field := range root.Fields {
		if field.Name == "version" {
			return nil, errors.New("metadata version must be an attribute")
		}
		if field.Name == "id" || field.Name == "importer" || field.Name == "importerVersion" {
			if critical[field.Name] {
				return nil, fmt.Errorf("ambiguous metadata field %s", field.Name)
			}
			critical[field.Name] = true
		}
	}
	for _, block := range root.Blocks {
		if block.Head == asset.SourceSettingsBlock {
			if p.settings != nil || block.Label != "" {
				return nil, errors.New("ambiguous audio metadata settings")
			}
			p.settings = block
		}
	}
	var editValue aetext.Value
	if p.settings != nil {
		for _, field := range p.settings.Attrs {
			if field.Name == editField {
				return nil, errors.New("audio edit must be a settings field")
			}
		}
		for _, block := range p.settings.Blocks {
			if block.Head == editField {
				return nil, errors.New("audio edit must not replace a settings block")
			}
		}
		seen := make(map[string]bool)
		for _, field := range p.settings.Fields {
			if seen[field.Name] {
				return nil, fmt.Errorf("ambiguous audio metadata setting %s", field.Name)
			}
			seen[field.Name] = true
			if field.Name == editField {
				p.present, editValue = true, field.Value
			}
		}
	}
	if (p.version == 1 && p.present) || (p.version == formatVersion && (!p.present || editValue.Kind != aetext.VString)) {
		return nil, errors.New("audio edit marker and metadata version disagree")
	}
	// Use the existing metadata validator on a temporary version-1 clone.
	// The version-2 bytes themselves are never handed to the legacy parser.
	compat := root.Clone()
	setVersion(compat, 1)
	if _, err := writeSize(compat, maxMetadataBytes); err != nil {
		return nil, err
	}
	p.meta, err = asset.ParseSourceMeta(aetext.Write(compat), sourcePath+asset.MetaSuffix)
	if err != nil {
		return nil, err
	}
	if p.meta.ID.IsZero() || p.meta.Importer != "audio" {
		return nil, errors.New("audio metadata needs its own AssetID and audio importer")
	}
	if p.present {
		p.edit, err = audioedit.Decode([]byte(editValue.Str))
		if err != nil {
			return nil, fmt.Errorf("invalid audio edit: %w", err)
		}
		if err := validateDocument(p.edit, p.meta, sourcePath); err != nil {
			return nil, err
		}
	}
	return p, nil
}

func validateDocument(doc audioedit.Document, meta *asset.SourceMeta, sourcePath string) error {
	if err := doc.Validate(); err != nil {
		return err
	}
	codecName := strings.TrimPrefix(strings.ToLower(path.Ext(sourcePath)), ".")
	if doc.Source.AssetID != meta.ID.String() || doc.Source.Path != sourcePath || doc.Source.Codec != codecName ||
		doc.Source.SHA256 != strings.ToLower(doc.Source.SHA256) || doc.Source.Channels > 2 {
		return errors.New("audio edit source identity or layout does not match its metadata")
	}
	return nil
}

func validSourcePath(sourcePath string) error {
	if !fs.ValidPath(sourcePath) || !filepath.IsLocal(sourcePath) || strings.ContainsAny(sourcePath, "\\:#") {
		return errors.New("invalid audio source path")
	}
	for _, part := range strings.Split(sourcePath, "/") {
		if strings.HasPrefix(part, ".") {
			return errors.New("hidden audio source path")
		}
	}
	switch strings.ToLower(path.Ext(sourcePath)) {
	case ".wav", ".ogg":
		return nil
	default:
		return errors.New("audio edit supports WAV and OGG sources only")
	}
}

func setVersion(root *aetext.Node, version int64) {
	for i := range root.Attrs {
		if root.Attrs[i].Name == "version" {
			root.Attrs[i].Value = aetext.Int(version)
			return
		}
	}
	root.Attrs = append(root.Attrs, aetext.Field{Name: "version", Value: aetext.Int(version)})
}

func boundedSyntax(raw []byte) error {
	var stack [maxNesting]byte
	depth := 0
	quoted, escaped := false, false
	for _, c := range raw {
		if quoted {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				quoted = false
			}
			continue
		}
		switch c {
		case '"':
			quoted = true
		case '{', '[', '(':
			if depth == len(stack) {
				return errors.New("audio sidecar nesting budget exceeded")
			}
			stack[depth] = c
			depth++
		case '}', ']', ')':
			if depth == 0 {
				return errors.New("invalid audio sidecar nesting")
			}
			depth--
			open := stack[depth]
			if (c == '}' && open != '{') || (c == ']' && open != '[') || (c == ')' && open != '(') {
				return errors.New("invalid audio sidecar nesting")
			}
		}
	}
	if quoted || depth != 0 {
		return errors.New("incomplete audio sidecar")
	}
	return nil
}
