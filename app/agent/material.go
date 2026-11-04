package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"slices"
	"strings"

	"aego/asset"
	"aego/core/errs"
	"aego/core/ids"
	"aego/format/aetext"
	"aego/render/material"
	"aego/render/shader"
	"aego/render/texture"
	"aego/scene/mutate"
)

func materialCommands() []*Command {
	file := Arg{Name: "file", Type: "string", Doc: "material file under assets/, ending in .aemat", Required: true, Pos: true}
	shaderArg := Arg{Name: "shader", Type: "string", Doc: "shader asset ID or project .aeshader path", Required: true, Pos: true}
	param := Arg{Name: "param", Type: "string", Doc: "declared material parameter name", Required: true, Pos: true}
	expected := Arg{Name: "expected", Type: "string", Doc: "sourceHash returned by material.get; refuse if the document changed"}
	return []*Command{
		{Name: "shader.schema", Doc: "declared parameters, typed defaults, domain and allowed variants; checks current source and unsaved includes", Args: []Arg{shaderArg}, Run: (*Session).shaderSchema},
		{Name: "shader.validate", Doc: "validate current source with the registered importer; reports schema/reference validation independently from GPU linking", Args: []Arg{shaderArg}, Run: (*Session).shaderSchema},
		{Name: "shader.create", Doc: "create a portable sprite shader through the registered asset importer", Write: true, Args: []Arg{{Name: "file", Type: "string", Doc: "new assets/*.aeshader path", Required: true, Pos: true}}, Run: (*Session).shaderCreate},
		{Name: "material.create", Doc: "create a material inheriting defaults from a shader", Write: true, Args: []Arg{file, shaderArg}, Run: (*Session).materialCreate},
		{Name: "material.get", Doc: "read explicit/inherited parameters, unknown data, variants and source revision from the current document", Args: []Arg{file}, Run: (*Session).materialGet},
		{Name: "material.validate", Doc: "validate material values, shader and texture references without changing the source", Args: []Arg{file}, Run: (*Session).materialValidate},
		{Name: "material.set", Doc: "set a typed explicit parameter as one semantic mutation", Write: true, Args: []Arg{file, param, argValue, expected}, Run: func(s *Session, a Args) (*Output, error) {
			return s.materialTx(a, mutate.Tx{Label: "Set Material Parameter", Ops: []mutate.Op{mutate.MaterialSet(a.Str("param"), a.Str("value"))}})
		}},
		{Name: "material.reset", Doc: "remove an explicit parameter and inherit the shader default", Write: true, Args: []Arg{file, param, expected}, Run: func(s *Session, a Args) (*Output, error) {
			return s.materialTx(a, mutate.Tx{Label: "Reset Material Parameter", Ops: []mutate.Op{mutate.MaterialReset(a.Str("param"))}})
		}},
		{Name: "material.variants", Doc: "select an allowed combination of comma-separated keywords; an empty string selects no keywords", Write: true,
			Args: []Arg{file, {Name: "keywords", Type: "string", Doc: "comma-separated shader keywords"}, expected}, Run: func(s *Session, a Args) (*Output, error) {
				var keywords []string
				for _, k := range strings.Split(a.Str("keywords"), ",") {
					if k = strings.TrimSpace(k); k != "" {
						keywords = append(keywords, k)
					}
				}
				return s.materialTx(a, mutate.Tx{Label: "Set Material Variant", Ops: []mutate.Op{mutate.MaterialVariants(keywords)}})
			}},
		{Name: "material.apply", Doc: "apply a material transaction atomically, using the same mutations and serialization as Inspector", Write: true,
			Args: []Arg{file, {Name: "tx", Type: "string", Doc: "tx version=1 with material-set/reset/shader/variants/blend operations", Required: true}, expected}, Run: func(s *Session, a Args) (*Output, error) {
				tx, err := mutate.ParseTx([]byte(a.Str("tx")), "material transaction")
				if err != nil {
					return nil, err
				}
				return s.materialTx(a, tx)
			}},
		{Name: "material.assign", Doc: "assign a compatible material to SpriteRenderer or Mesh2D through the scene mutation service", Write: true,
			Args: []Arg{argFile, argNode, {Name: "material", Type: "string", Doc: "material asset ID or project path", Required: true, Pos: true}}, Run: (*Session).materialAssign},
	}
}

func sourceHash(src []byte) string {
	sum := sha256.Sum256(src)
	return hex.EncodeToString(sum[:])
}

func (s *Session) sourceOverlay(rel string) ([]byte, bool) {
	if s.host != nil {
		if text, ok := s.host.ScriptText(rel); ok {
			return []byte(text), true
		}
	}
	return nil, false
}

func (s *Session) currentShader(id ids.AssetID) (*asset.ShaderArtifact, error) {
	if data, ok := asset.BuiltinShader(id); ok {
		return asset.DecodeShader(data)
	}
	rel, ok := s.Pipeline.PathOf(id)
	if !ok || path.Ext(rel) != ".aeshader" {
		return nil, errs.New(errs.AssetNotFound, "shader asset is missing: "+id.String())
	}
	text, err := s.scriptText(rel)
	if err != nil {
		return nil, err
	}
	result, err := s.Pipeline.Preview(rel, []byte(text), s.sourceOverlay)
	if err != nil {
		return nil, err
	}
	for _, a := range result.Artifacts {
		if a.Kind == asset.ArtShader {
			return asset.DecodeShader(a.Data)
		}
	}
	return nil, errs.New(errs.AssetBadArtifact, "the shader importer produced no AESH artifact")
}

func (s *Session) materialShader(id ids.AssetID) (*material.Shader, bool) {
	a, err := s.currentShader(id)
	if err != nil {
		return nil, false
	}
	return a.Runtime(id, s.label(id)), true
}

type materialParamView struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Default  any    `json:"default"`
	Explicit bool   `json:"explicit"`
	Value    string `json:"value,omitempty"`
	Instance bool   `json:"instanceOverride"`
	Color    bool   `json:"color,omitempty"`
	Normal   bool   `json:"normal,omitempty"`
	Emission bool   `json:"emission,omitempty"`
	Offset   int    `json:"offset"`
	Slot     int    `json:"samplerSlot,omitempty"`
}

func paramView(p shader.Param) materialParamView {
	var value any
	switch p.Kind {
	case shader.KindTexture:
		value = nil
	case shader.KindBool:
		value = p.Default[0] != 0
	case shader.KindInt:
		value = p.IntDefault
	case shader.KindFloat:
		value = p.Default[0]
	default:
		value = slices.Clone(p.Default[:p.Kind.Lanes()])
	}
	return materialParamView{Name: p.Name, Type: p.Kind.GLSL(), Default: value, Instance: p.Instance, Color: p.Color, Normal: p.Normal, Emission: p.Emission, Offset: p.Offset, Slot: p.Slot}
}

func (s *Session) shaderSchema(a Args) (*Output, error) {
	id, err := s.asset(a.Str("shader"))
	if err != nil {
		return nil, err
	}
	artifact, err := s.currentShader(id)
	if err != nil {
		return nil, err
	}
	params := make([]materialParamView, 0, len(artifact.Params))
	for _, p := range artifact.Params {
		params = append(params, paramView(p))
	}
	variants := make([][]string, 0, len(artifact.Variants))
	for _, v := range artifact.Variants {
		keys := []string{}
		for i, key := range artifact.Keywords {
			if v.Mask&(1<<i) != 0 {
				keys = append(keys, key)
			}
		}
		variants = append(variants, keys)
	}
	return &Output{Value: map[string]any{
		"id": id.String(), "domain": artifact.Domain.String(), "blend": artifact.Blend.String(),
		"abi": shader.ABI, "parameters": params, "keywords": artifact.Keywords, "variants": variants,
		"schemaChecked":    artifact.Validation&shader.SchemaChecked != 0,
		"referenceChecked": artifact.Validation&shader.ReferenceChecked != 0,
		"gl33Linked":       false, "webgl2Linked": false,
	}}, nil
}

func (s *Session) materialDocument(a Args) (string, []byte, *mutate.MaterialDocument, error) {
	rel, err := s.rel(a.Str("file"))
	if err != nil {
		return "", nil, nil, err
	}
	if path.Ext(rel) != ".aemat" {
		return "", nil, nil, errs.New(errs.VFSBadPath, "material commands take .aemat files")
	}
	text, err := s.scriptText(rel)
	if err != nil {
		return "", nil, nil, err
	}
	src := []byte(text)
	if expected := a.Str("expected"); expected != "" && expected != sourceHash(src) {
		return "", nil, nil, errs.New(errs.EditorCommand, rel+" changed; read material.get again")
	}
	doc, err := mutate.LoadMaterial(src, rel, s.materialShader)
	return rel, src, doc, err
}

func (s *Session) materialGet(a Args) (*Output, error) {
	rel, src, doc, err := s.materialDocument(a)
	if err != nil {
		return nil, err
	}
	d := doc.Doc()
	params := []materialParamView{}
	unknown := map[string]string{}
	sh, hasShader := doc.Shader()
	if hasShader {
		for _, p := range sh.Schema.Params {
			v := paramView(p)
			raw, explicit := d.Value(p.Name)
			if !explicit {
				for _, former := range p.Former {
					if raw, explicit = d.Value(former); explicit {
						break
					}
				}
			}
			if explicit {
				v.Explicit, v.Value = true, mutate.FormatValue(raw)
			}
			params = append(params, v)
		}
	}
	for _, v := range d.Values {
		if hasShader {
			if _, exists := sh.Schema.Param(v.Name); exists {
				continue
			}
		}
		if raw, exists := d.Value(v.Name); exists {
			unknown[v.Name] = mutate.FormatValue(raw)
		}
	}
	blend := d.Blend
	if !d.HasBlend && hasShader {
		blend = sh.Blend
	}
	return &Output{Problems: doc.Problems(), Value: map[string]any{
		"file": rel, "shader": d.Shader.String(), "sourceHash": sourceHash(src), "parameters": params,
		"unknown": unknown, "variants": d.Variants, "blendExplicit": d.HasBlend, "blend": blend.String(),
	}}, nil
}

func (s *Session) materialValidate(a Args) (*Output, error) {
	_, _, doc, err := s.materialDocument(a)
	if err != nil {
		return nil, err
	}
	problems := doc.Problems()
	failed := map[ids.AssetID]string{}
	for _, a := range s.Pipeline.Assets() {
		if a.Error != "" {
			failed[a.ID] = a.Error
		}
	}
	names := make([]string, 0, len(doc.Doc().Textures))
	for name := range doc.Doc().Textures {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		id := doc.Doc().Textures[name]
		if id.IsZero() {
			continue
		}
		e, exists := s.Pipeline.Entry(id)
		sh, _ := doc.Shader()
		var param *shader.Param
		declared := false
		if sh != nil {
			param, declared = sh.Schema.Param(name)
		}
		regionAllowed := declared && (param.Normal || param.Emission)
		b, builtin := asset.LookupBuiltin(id)
		if (!builtin || b.Kind != asset.ArtTexture) && (!exists || e.Missing || (e.Kind != asset.ArtTexture && e.Kind != asset.ArtAtlasPage && !(regionAllowed && e.Kind == asset.ArtSpriteRegion))) {
			problems = append(problems, mutate.Problem{Error: true, Message: "MAT004: texture " + name + " is unresolved or has an incompatible region binding: " + id.String()})
		} else if reason := failed[id]; reason != "" {
			problems = append(problems, mutate.Problem{Error: true, Message: "MAT004: texture " + name + " has a failed current import: " + reason})
		} else if !builtin {
			info, valid := s.Pipeline.TextureSourceInfo(id, s.sourceOverlay)
			if declared && (param.Normal || param.Emission) {
				normal := info.Role == asset.RoleNormal || (info.Role == asset.RoleHeight && info.GenerateNormal)
				emission := info.Role == asset.RoleEmissionRGB || info.Role == asset.RoleEmissionMask
				if !valid || (param.Normal && !normal) || (param.Emission && !emission) {
					problems = append(problems, mutate.Problem{Error: true, Message: "MAT004: incompatible surface import role for " + name + ": " + id.String()})
				}
			}
			if err := s.Pipeline.VerifyCurrent(id); err != nil {
				problems = append(problems, mutate.Problem{Error: true, Message: "MAT004: texture " + name + " is not current: " + err.Error()})
			}
		}
	}
	return &Output{Problems: problems}, nil
}

func (s *Session) materialTx(a Args, tx mutate.Tx) (*Output, error) {
	rel, before, doc, err := s.materialDocument(a)
	if err != nil {
		return nil, err
	}
	var res *mutate.Result
	if a.Bool("dry-run") {
		res, err = doc.DryRun(tx)
	} else {
		res, err = doc.Apply(tx)
	}
	if err != nil {
		return nil, err
	}
	out := &Output{DryRun: res.DryRun, Changes: res.Changes, Problems: res.Problems}
	if res.DryRun {
		return out, nil
	}
	if s.host != nil {
		err = s.host.ApplyScript(ScriptEdit{Rel: rel, Before: string(before), After: string(doc.Bytes())}, tx.Label)
		out.Notes = append(out.Notes, "applied to the editor document as one Undo step")
	} else {
		out.Files, err = s.save(rel, before, doc.Bytes())
	}
	return out, err
}

func (s *Session) createMaterialSource(a Args, ext string, src []byte) (*Output, error) {
	rel, err := s.rel(a.Str("file"))
	if err != nil {
		return nil, err
	}
	if path.Ext(rel) != ext || !strings.HasPrefix(rel, "assets/") {
		return nil, errs.New(errs.VFSBadPath, "create under assets/ with extension "+ext)
	}
	if s.exists(rel) {
		return nil, errs.New(errs.VFSBadPath, rel+" already exists")
	}
	if _, err := s.Pipeline.Preview(rel, src, s.sourceOverlay); err != nil {
		return nil, err
	}
	out := &Output{Value: map[string]any{"file": rel, "source": string(src)}}
	if a.Bool("dry-run") {
		out.DryRun = true
		return out, nil
	}
	if s.host != nil {
		if err := s.host.ApplyScript(ScriptEdit{Rel: rel, New: true, After: string(src)}, "Create "+path.Base(rel)); err != nil {
			return nil, err
		}
		if _, _, err := s.Pipeline.Scan(); err != nil {
			return nil, err
		}
		id, exists := s.Pipeline.Lookup(rel)
		if !exists {
			return nil, errs.New(errs.AssetNotFound, "created source has no registered importer")
		}
		batch, err := s.Pipeline.Import(id)
		if err != nil {
			return nil, err
		}
		if batch != nil && batch.Failed[id] != nil {
			return nil, batch.Failed[id]
		}
		out.Created = []string{id.String()}
	} else {
		id, err := s.create(rel, src)
		if err != nil {
			return nil, err
		}
		out.Created = []string{id.String()}
	}
	out.Files = []string{rel}
	return out, nil
}

func (s *Session) shaderCreate(a Args) (*Output, error) {
	src := []byte("#pragma aego abi 1\n#include \"engine/sprite.glsl\"\n\n#pragma material\nuniform float Opacity; // @instance @range(0, 1) @default(1)\n#pragma end\n\nvoid fragment() {\n    aego_out = aego_sample() * Opacity;\n}\n")
	return s.createMaterialSource(a, ".aeshader", src)
}

func (s *Session) materialCreate(a Args) (*Output, error) {
	id, err := s.asset(a.Str("shader"))
	if err != nil {
		return nil, err
	}
	if _, err := s.currentShader(id); err != nil {
		return nil, err
	}
	src := []byte(fmt.Sprintf("material version=1 shader=asset(\"%s\") {\n  params {}\n}\n", id.String()))
	return s.createMaterialSource(a, ".aemat", src)
}

func (s *Session) materialAssign(a Args) (*Output, error) {
	id, err := s.asset(a.Str("material"))
	if err != nil {
		return nil, err
	}
	var sh *material.Shader
	var ok bool
	if data, builtin := asset.BuiltinMaterial(id); builtin {
		doc, err := asset.DecodeMaterial(data, s.label(id))
		if err != nil {
			return nil, err
		}
		sh, ok = s.materialShader(doc.Shader)
	} else {
		rel, exists := s.Pipeline.PathOf(id)
		if !exists {
			return nil, errs.New(errs.AssetNotFound, "material source is missing")
		}
		_, _, doc, err := s.materialDocument(Args{"file": rel})
		if err != nil {
			return nil, err
		}
		sh, ok = doc.Shader()
	}
	if !ok {
		return nil, errs.New(errs.AssetNotFound, "the material shader is unavailable")
	}
	component := "engine.SpriteRenderer"
	if sh.Domain == shader.DomainMesh2D {
		component = "engine.Mesh2D"
	} else if sh.Domain != shader.DomainSprite {
		return nil, errs.New(errs.EditorProps, "material domain cannot be assigned to a 2D node")
	}
	return s.sceneTx(a, "Assign Material", mutate.SetValue(a.Str("node"), component+".material", aetext.Value{Kind: aetext.VAsset, Str: id.String()}))
}

// Source-only command sessions expose the same material definitions as runtime
// resolvers. They never create GPU handles or claim that reference validation
// is a real device link.
func (r sources) Shader(id ids.AssetID) (*material.Shader, bool) {
	data, builtin := asset.BuiltinShader(id)
	if !builtin {
		rel, ok := r.pl.PathOf(id)
		if !ok {
			return nil, false
		}
		src, ok := r.read(id)
		if !ok {
			return nil, false
		}
		result, err := r.pl.Preview(rel, src, nil)
		if err != nil {
			return nil, false
		}
		for _, a := range result.Artifacts {
			if a.Kind == asset.ArtShader {
				data = a.Data
				break
			}
		}
	}
	a, err := asset.DecodeShader(data)
	if err != nil {
		return nil, false
	}
	return a.Runtime(id, id.String()), true
}

func (r sources) Material(id ids.AssetID) (*material.Material, bool) {
	var doc *asset.MaterialDoc
	var err error
	if data, builtin := asset.BuiltinMaterial(id); builtin {
		doc, err = asset.DecodeMaterial(data, id.String())
	} else {
		src, ok := r.read(id)
		if !ok {
			return nil, false
		}
		doc, err = asset.ParseMaterial(src, id.String())
	}
	if err != nil {
		return nil, false
	}
	sh, ok := r.Shader(doc.Shader)
	if !ok {
		return nil, false
	}
	m, _ := doc.Build(id.String(), sh, func(tid ids.AssetID) (material.Tex, bool) {
		if b, ok := asset.LookupBuiltin(tid); ok {
			return material.Tex{}, b.Kind == asset.ArtTexture
		}
		e, ok := r.pl.Entry(tid)
		info, valid := r.pl.TextureSourceInfo(tid, nil)
		mode := texture.EmissionNone
		switch info.Role {
		case asset.RoleEmissionRGB:
			mode = texture.EmissionRGB
		case asset.RoleEmissionMask:
			mode = texture.EmissionMask
		}
		return material.Tex{RegionOnly: e.Kind == asset.ArtSpriteRegion, NormalMap: valid && (info.Role == asset.RoleNormal || (info.Role == asset.RoleHeight && info.GenerateNormal)), EmissionMode: mode}, ok && !e.Missing && (e.Kind == asset.ArtTexture || e.Kind == asset.ArtAtlasPage || e.Kind == asset.ArtSpriteRegion)
	})
	if m != nil {
		m.ID = id
	}
	return m, m != nil
}
