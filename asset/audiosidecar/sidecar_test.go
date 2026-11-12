package audiosidecar

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"aego/asset"
	"aego/audio/edit"
	"aego/core/ids"
	"aego/format/aetext"
)

func sidecarFixture(t *testing.T) ([]byte, string, audioedit.Document) {
	t.Helper()
	id := ids.NewAssetID()
	path := "assets/sound.wav"
	meta := []byte(fmt.Sprintf(`meta version=1 future="keep" {
 id="%s" importer="audio" importerVersion=1 extension={token="keep"}
 settings { stream=auto gain=1 extra=[1, 2, 3] nested "future" { token="stay" } }
 unknown "section" { text="hello" }
}`, id.String()))
	digest := sha256.Sum256([]byte("synthetic source"))
	doc, err := audioedit.New(audioedit.Source{AssetID: id.String(), Path: path, SHA256: fmt.Sprintf("%x", digest),
		Codec: "wav", SampleRate: 48000, Channels: 2, Frames: 128})
	if err != nil {
		t.Fatal(err)
	}
	doc.Trim = audioedit.Range{Start: 8, End: 120}
	doc.Loop = &audioedit.Range{Start: 12, End: 110}
	doc.Operations = []audioedit.Operation{{Kind: audioedit.Gain, Region: audioedit.Range{Start: 8, End: 120}, Value: .5}}
	doc.Markers = []audioedit.Marker{{ID: "m1", Label: "кадр", Frame: 40}}
	return meta, path, doc
}

func TestSidecarRoundTripPreservesMetadataAndOldReadersRefuse(t *testing.T) {
	meta, path, doc := sidecarFixture(t)
	before := bytes.Clone(meta)
	if _, present, err := Decode(meta, path); err != nil || present {
		t.Fatalf("ordinary metadata did not remain ordinary: present=%v err=%v", present, err)
	}
	encoded, err := Encode(meta, path, doc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(meta, before) {
		t.Fatal("encoding mutated the caller's source metadata")
	}
	if _, err := asset.ParseSourceMeta(encoded, path+asset.MetaSuffix); err == nil {
		t.Fatal("current/legacy metadata reader silently accepted the EDL")
	}
	decoded, present, err := Decode(encoded, path)
	if err != nil || !present || !reflect.DeepEqual(decoded, doc) {
		t.Fatalf("EDL round-trip mismatch: present=%v error=%v", present, err)
	}
	n, err := aetext.Parse(encoded, path+asset.MetaSuffix)
	if err != nil {
		t.Fatal(err)
	}
	version, ok := n.Attr("version")
	if !ok || version.Kind != aetext.VInt || version.Int != 2 {
		t.Fatal("edited sidecar lacks a mandatory version 2 marker")
	}
	for _, preserved := range []string{"future=", "extension=", "extra=", `nested "future"`, `unknown "section"`} {
		if !bytes.Contains(encoded, []byte(preserved)) {
			t.Fatalf("lost unknown metadata %q: %s", preserved, encoded)
		}
	}
	updated := doc.Clone()
	updated.Operations = append(updated.Operations, audioedit.Operation{Kind: audioedit.Reverse, Region: audioedit.Range{Start: 20, End: 80}})
	again, err := Encode(encoded, path, updated)
	if err != nil {
		t.Fatal(err)
	}
	decoded, present, err = Decode(again, path)
	if err != nil || !present || !reflect.DeepEqual(decoded, updated) {
		t.Fatalf("updated EDL mismatch: present=%v error=%v", present, err)
	}
	if _, err := asset.ParseSourceMeta(again, path+asset.MetaSuffix); err == nil {
		t.Fatal("updating the EDL downgraded its fail-closed marker")
	}
}

func TestSidecarRejectsUnsafeIdentityAndLossyUpdates(t *testing.T) {
	meta, path, doc := sidecarFixture(t)
	encoded, err := Encode(meta, path, doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, reason := range []string{"asset id", "source path", "digest", "codec", "channels", "traversal", "source replacement", "wrong importer", "legacy reserved field", "too many operations"} {
		t.Run(reason, func(t *testing.T) {
			candidate, source, raw := doc.Clone(), path, bytes.Clone(meta)
			switch reason {
			case "asset id":
				candidate.Source.AssetID = ids.NewAssetID().String()
			case "source path":
				candidate.Source.Path = "assets/other.wav"
			case "digest":
				candidate.Source.SHA256 = strings.ToUpper(candidate.Source.SHA256)
			case "codec":
				candidate.Source.Codec = "ogg"
			case "channels":
				candidate.Source.Channels = 3
			case "traversal":
				source, candidate.Source.Path = "assets/../sound.wav", "assets/../sound.wav"
			case "source replacement":
				raw = encoded
				candidate.Source.SHA256 = strings.Repeat("a", 64)
			case "wrong importer":
				raw = bytes.Replace(raw, []byte(`importer="audio"`), []byte(`importer="texture"`), 1)
			case "legacy reserved field":
				raw = bytes.Replace(raw, []byte("settings {"), []byte(`settings { audioEdit="other owner's field" `), 1)
			case "too many operations":
				candidate.Operations = make([]audioedit.Operation, 4097)
			}
			if _, err := Encode(raw, source, candidate); err == nil {
				t.Fatalf("accepted unsafe %s", reason)
			}
		})
	}
}

func TestSidecarRejectsMalformedOrAmbiguousMetadata(t *testing.T) {
	meta, path, doc := sidecarFixture(t)
	encoded, err := Encode(meta, path, doc)
	if err != nil {
		t.Fatal(err)
	}
	n, err := aetext.Parse(encoded, path+asset.MetaSuffix)
	if err != nil {
		t.Fatal(err)
	}
	settings := n.Block(asset.SourceSettingsBlock, "")
	edit, ok := settings.Field(editField)
	if !ok || edit.Kind != aetext.VString {
		t.Fatal("fixture is missing its edit document")
	}
	variant := func(value *aetext.Value) []byte {
		t.Helper()
		clone := n.Clone()
		block := clone.Block(asset.SourceSettingsBlock, "")
		if value == nil {
			block.RemoveField(editField)
		} else {
			block.SetField(editField, *value)
		}
		return aetext.Write(clone)
	}
	wrongType := aetext.Ident("unsupported")
	unknownJSON := aetext.String(strings.TrimSuffix(edit.Str, "}") + `,"future":true}`)
	duplicateJSON := aetext.String(strings.Replace(edit.Str, "{", `{"Schema":1,`, 1))
	for _, tc := range []struct {
		name string
		raw  []byte
	}{
		{"duplicate-version", bytes.Replace(encoded, []byte("version=2"), []byte("version=1 version=2"), 1)},
		{"version-field", bytes.Replace(encoded, []byte("importerVersion=1"), []byte("importerVersion=1 version=1"), 1)},
		{"future-version", bytes.Replace(encoded, []byte("version=2"), []byte("version=3"), 1)},
		{"duplicate-settings", bytes.Replace(encoded, []byte("  settings {"), []byte("  settings {}\n  settings {"), 1)},
		{"legacy-edl", bytes.Replace(encoded, []byte("version=2"), []byte("version=1"), 1)},
		{"wrong-edl-type", variant(&wrongType)},
		{"missing-edl", variant(nil)},
		{"unknown-edl-field", variant(&unknownJSON)},
		{"duplicate-edl-key", variant(&duplicateJSON)},
		{"duplicate-id", bytes.Replace(encoded, []byte("  id=\""), []byte("  id=\"00000000000000000000000000000001\"\n  id=\""), 1)},
		{"deep-unknown", []byte("meta version=1 { x=" + strings.Repeat("[", 80) + "0" + strings.Repeat("]", 80) + " }")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := Decode(tc.raw, path); err == nil {
				t.Fatal("accepted malformed sidecar")
			}
			if _, err := Encode(tc.raw, path, doc); err == nil {
				t.Fatal("overwrote malformed sidecar")
			}
		})
	}
}

func TestSidecarHandlesMissingSettingsAndQuotedBrackets(t *testing.T) {
	_, path, doc := sidecarFixture(t)
	meta := &aetext.Node{Head: "meta", Fields: []aetext.Field{
		{Name: "id", Value: aetext.String(doc.Source.AssetID)},
		{Name: "importer", Value: aetext.String("audio")},
		{Name: "note", Value: aetext.String(strings.Repeat("[{(\\\")}]", 80))},
	}}
	raw := aetext.Write(meta)
	encoded, err := Encode(raw, path, doc)
	if err != nil {
		t.Fatal("valid quoted delimiters or missing settings were rejected", err)
	}
	got, present, err := Decode(encoded, path)
	if err != nil || !present || !reflect.DeepEqual(got, doc) {
		t.Fatalf("new settings round-trip: present=%v err=%v", present, err)
	}
	if _, err := asset.ParseSourceMeta(encoded, path+asset.MetaSuffix); err == nil {
		t.Fatal("legacy parser accepted newly created edit settings")
	}
}
