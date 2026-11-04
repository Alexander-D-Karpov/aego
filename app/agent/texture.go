package agent

import (
	"bytes"
	"fmt"
	"image/png"
	"os"
	"path"
	"strings"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/errs"
	"aego/core/ids"
	"aego/format/aetext"
	"aego/scene/mutate"
)

func textureCommands() []*Command {
	file := Arg{Name: "file", Type: "string", Doc: "source .png path or its .meta; frames share one source document", Required: true, Pos: true}
	expected := Arg{Name: "expected", Type: "string", Doc: "sourceHash from texture.get; checks metadata and image bytes"}
	frame := Arg{Name: "frame", Type: "string", Doc: "stable frame AssetID; absent selects the whole source"}
	channel := Arg{Name: "channel", Type: "string", Doc: "normal, height or emission", Required: true, Pos: true}
	return []*Command{
		{Name: "texture.schema", Doc: "registered import roles/settings and source-frame channel contract", Run: func(s *Session, a Args) (*Output, error) {
			return &Output{Value: map[string]any{"settings": textureSettingSchema(), "channels": []string{"normal", "height", "emission"}, "frameIdentity": "AssetID", "sourceOrigin": "top-left", "pivot": "normalized logical canvas; node Origin remains explicit"}}, nil
		}},
		{Name: "texture.get", Doc: "read current dirty metadata, stable frames, source size, roles and channel references", Args: []Arg{file}, Run: (*Session).textureGet},
		{Name: "texture.validate", Doc: "check source metadata and prepare a private import candidate without changing cache or GPU resources", Args: []Arg{file}, Run: (*Session).textureValidate},
		{Name: "texture.set", Doc: "set an import setting using the shared semantic mutation service", Write: true, Args: []Arg{file, {Name: "setting", Type: "string", Required: true, Pos: true}, argValue, expected}, Run: func(s *Session, a Args) (*Output, error) {
			raw := a.Str("value")
			var value aetext.Value
			var err error
			enum := false
			if spec, ok := asset.TextureSetting(a.Str("setting")); ok && spec.Type == "enum" {
				for _, choice := range spec.Choices {
					if raw == choice {
						value = aetext.String(raw)
						enum = true
						break
					}
				}
			}
			if !enum {
				value, err = mutate.ParseValue(raw)
				if err != nil {
					return nil, err
				}
			}
			return s.textureTx(a, mutate.Tx{Label: "Set Texture Import Setting", Ops: []mutate.Op{mutate.TextureSetting(a.Str("setting"), value)}})
		}},
		{Name: "texture.reset", Doc: "remove an explicit import setting and inherit the importer default", Write: true, Args: []Arg{file, {Name: "setting", Type: "string", Required: true, Pos: true}, expected}, Run: func(s *Session, a Args) (*Output, error) {
			return s.textureTx(a, mutate.Tx{Label: "Reset Texture Import Setting", Ops: []mutate.Op{mutate.TextureSettingReset(a.Str("setting"))}})
		}},
		{Name: "texture.channel", Doc: "assign a normal/height/emission source or frame by stable AssetID; unresolved IDs are retained with diagnostics", Write: true, Args: []Arg{file, channel, {Name: "asset", Type: "string", Doc: "target AssetID or project source path", Required: true, Pos: true}, frame, expected}, Run: func(s *Session, a Args) (*Output, error) {
			target, err := s.textureTarget(a.Str("asset"))
			if err != nil {
				return nil, err
			}
			f, err := textureFrameArg(a)
			if err != nil {
				return nil, err
			}
			return s.textureTx(a, mutate.Tx{Label: "Assign Surface Channel", Ops: []mutate.Op{mutate.TextureChannel(f, a.Str("channel"), target)}})
		}},
		{Name: "texture.channel-reset", Doc: "remove the explicit channel mapping without deleting source assets", Write: true, Args: []Arg{file, channel, frame, expected}, Run: func(s *Session, a Args) (*Output, error) {
			f, err := textureFrameArg(a)
			if err != nil {
				return nil, err
			}
			return s.textureTx(a, mutate.Tx{Label: "Reset Surface Channel", Ops: []mutate.Op{mutate.TextureChannelReset(f, a.Str("channel"))}})
		}},
		{Name: "texture.apply", Doc: "atomic typed texture metadata transaction; same validation and Undo rules as Inspector", Write: true, Args: []Arg{file, {Name: "tx", Type: "string", Doc: "tx version=1 with texture-setting/channel/frame operations", Required: true}, expected}, Run: func(s *Session, a Args) (*Output, error) {
			tx, err := mutate.ParseTx([]byte(a.Str("tx")), "texture transaction")
			if err != nil {
				return nil, err
			}
			return s.textureTx(a, tx)
		}},
	}
}
func textureFrameArg(a Args) (ids.AssetID, error) {
	if a.Str("frame") == "" {
		return ids.AssetID{}, nil
	}
	return ids.ParseAssetID(a.Str("frame"))
}
func (s *Session) textureTarget(value string) (ids.AssetID, error) {
	if id, err := ids.ParseAssetID(value); err == nil {
		return id, nil
	}
	return s.asset(value)
}

type textureAuthoring struct {
	source, meta  string
	image, before []byte
	doc           *mutate.TextureDocument
	hash          string
}

func (s *Session) textureDocument(a Args) (textureAuthoring, error) {
	var out textureAuthoring
	rel, err := s.rel(a.Str("file"))
	if err != nil {
		return out, err
	}
	rel = strings.TrimSuffix(rel, pipeline.MetaSuffix)
	rel, _, _ = strings.Cut(rel, pipeline.SubSep)
	if path.Ext(rel) != ".png" {
		return out, errs.New(errs.VFSBadPath, "texture commands require an imported PNG source")
	}
	encoded, err := s.scriptText(rel + pipeline.MetaSuffix)
	if err != nil {
		return out, err
	}
	image, err := os.ReadFile(s.Pipeline.Abs(rel))
	if err != nil {
		return out, err
	}
	config, err := png.DecodeConfig(bytes.NewReader(image))
	if err != nil {
		return out, err
	}
	before := []byte(encoded)
	combined := make([]byte, 0, len(before)+len(image))
	combined = append(combined, before...)
	combined = append(combined, image...)
	hash := sourceHash(combined)
	if expected := a.Str("expected"); expected != "" && expected != hash {
		return out, errs.New(errs.EditorCommand, rel+" or its metadata changed; read texture.get again")
	}
	env := mutate.TextureEnv{Width: config.Width, Height: config.Height, Resolve: func(id ids.AssetID) (asset.TextureSourceInfo, bool) {
		return s.Pipeline.TextureSourceInfo(id, s.sourceOverlay)
	}}
	doc, err := mutate.LoadTexture(before, rel+pipeline.MetaSuffix, env)
	if err != nil {
		return out, err
	}
	return textureAuthoring{source: rel, meta: rel + pipeline.MetaSuffix, image: image, before: before, doc: doc, hash: hash}, nil
}
func (s *Session) textureGet(a Args) (*Output, error) {
	state, err := s.textureDocument(a)
	if err != nil {
		return nil, err
	}
	m := state.doc.Doc()
	config, _ := png.DecodeConfig(bytes.NewReader(state.image))
	role, _ := asset.TextureRoleOf(state.source, m.Settings)
	settings := map[string]string{}
	for key, value := range m.Settings {
		settings[key] = mutate.FormatValue(value)
	}
	frames := []map[string]any{}
	for _, f := range m.Subs {
		frames = append(frames, map[string]any{"id": f.ID.String(), "name": f.Name, "rect": f.Rect, "pivot": f.EffectivePivot(), "pivotExplicit": f.HasPivot, "channels": textureChannels(m, f.ID)})
	}
	return &Output{Problems: state.doc.Problems(), Value: map[string]any{"id": m.ID.String(), "source": state.source, "metadata": state.meta, "sourceHash": state.hash, "metadataHash": sourceHash(state.before), "imageHash": sourceHash(state.image), "sourceSize": [2]int{config.Width, config.Height}, "sourceOrigin": "top-left", "role": role, "pivot": m.EffectivePivot(), "pivotExplicit": m.HasPivot, "settings": settings, "frames": frames, "channels": textureChannels(m, m.ID)}}, nil
}
func textureChannels(m *asset.SourceMeta, id ids.AssetID) map[string]any {
	out := map[string]any{}
	for _, channel := range []string{"normal", "height", "emission"} {
		ref, explicit, err := asset.TextureChannel(m, id, channel)
		entry := map[string]any{"explicit": explicit, "asset": ref.String()}
		if err != nil {
			entry["error"] = err.Error()
		}
		out[channel] = entry
	}
	return out
}
func (s *Session) textureValidate(a Args) (*Output, error) {
	state, err := s.textureDocument(a)
	if err != nil {
		return nil, err
	}
	problems := state.doc.Problems()
	result, err := s.Pipeline.PreviewWithMetadata(state.source, state.image, state.doc.Bytes(), s.sourceOverlay)
	if err != nil {
		problems = append(problems, mutate.Problem{Error: true, Message: "TEX003: " + err.Error()})
	}
	checked := err == nil
	var kind any
	if result != nil {
		kind = result.Kind
	}
	return &Output{Problems: problems, Value: map[string]any{"schemaChecked": true, "importChecked": checked, "gpuLinked": false, "artifactKind": kind, "sourceHash": state.hash}}, nil
}
func (s *Session) textureTx(a Args, tx mutate.Tx) (*Output, error) {
	state, err := s.textureDocument(a)
	if err != nil {
		return nil, err
	}
	var res *mutate.Result
	if a.Bool("dry-run") {
		res, err = state.doc.DryRun(tx)
	} else {
		res, err = state.doc.Apply(tx)
	}
	if err != nil {
		return nil, err
	}
	out := &Output{DryRun: res.DryRun, Changes: res.Changes, Problems: res.Problems}
	if res.DryRun {
		return out, nil
	}
	if s.host != nil {
		err = s.host.ApplyScript(ScriptEdit{Rel: state.meta, Before: string(state.before), After: string(state.doc.Bytes())}, tx.Label)
		out.Notes = append(out.Notes, "applied to the shared source metadata document as one Undo step; Save imports the new generation")
	} else {
		if err := s.writable(); err != nil {
			return nil, err
		}
		out.Files, err = s.save(state.meta, state.before, state.doc.Bytes())
		if err == nil {
			batch, importErr := s.Pipeline.ReloadMetadata(state.doc.Doc().ID)
			if importErr != nil {
				out.Problems = append(out.Problems, mutate.Problem{Error: true, Message: "TEX003: " + importErr.Error()})
			} else if batch != nil {
				for _, failure := range batch.Failed {
					out.Problems = append(out.Problems, mutate.Problem{Error: true, Message: fmt.Sprint(failure)})
				}
			}
		}
	}
	return out, err
}

func textureSettingSchema() []map[string]any {
	out := []map[string]any{}
	for _, s := range asset.TextureSettings() {
		out = append(out, map[string]any{"name": s.Name, "type": s.Type, "default": mutate.FormatValue(s.Default), "choices": s.Choices, "roles": s.Roles})
	}
	return out
}
