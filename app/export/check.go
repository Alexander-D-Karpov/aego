package export

import (
	"path/filepath"
	"strings"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/app/export/icon"
	"aego/asset"
	scriptImporter "aego/asset/importers/script"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/engine/module"
	"aego/engine/problems"
	"aego/modules/particles"
	"aego/project"
	"aego/render/mesh"
	"aego/render/texture"
	"aego/runtime/bundle"
	"aego/scene/sceneio"
	"aego/script/bind"
)

const (
	CodeStartScene = "EXP001"
	CodeScripts    = "EXP002"
	CodeImport     = "EXP003"
	CodeMissing    = "EXP004"
	CodeIcon       = "EXP005"
	CodeMeta       = "EXP006"
	CodeScene      = "EXP007"
	CodeUnknown    = "EXP008"
	CodeInternal   = "EXP009"
)

func diag(code string, sev problems.Severity, msg, file string) problems.Diagnostic {
	return problems.Diagnostic{
		Owner: Owner, Code: code, Severity: sev, Message: msg,
		Subject:    problems.Subject{Kind: problems.SubjectFile, File: file},
		BlocksPlay: sev == problems.Error,
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func Blocking(ds []problems.Diagnostic) int {
	n := 0
	for _, d := range ds {
		if d.Severity == problems.Error {
			n++
		}
	}
	return n
}

func resolves(idx *asset.Index, id ids.AssetID) bool {
	if _, ok := asset.LookupBuiltin(id); ok {
		return true
	}
	e, ok := idx.Lookup(id)
	return ok && !e.Missing
}

func Check(p *project.Project, s *Snapshot, profile string, partial bool) []problems.Diagnostic {
	var out []problems.Diagnostic
	out = append(out, s.capture...)
	out = append(out, checkMaterialSources(s)...)
	out = append(out, checkMaterialArtifacts(s)...)
	out = append(out, checkNormalAssociations(s)...)
	out = append(out, checkPresetSources(s)...)
	seen := map[string]bool{}
	for _, a := range s.assets {
		switch {
		case a.Error == "":
		case a.Importer == scriptImporter.ImporterID:
			if !seen[a.Error] {
				seen[a.Error] = true
				out = append(out, diag(CodeScripts, problems.Error, "scripts do not compile: "+firstLine(a.Error), a.Path))
			}
		default:
			out = append(out, diag(CodeImport, problems.Error, "import failed: "+firstLine(a.Error), a.Path))
		}
	}
	for _, is := range s.issues {
		if is.Kind == pipeline.MetaDuplicate {
			out = append(out, diag(CodeMeta, problems.Error, is.Message(), is.Path))
		}
	}
	switch e, ok := s.idx.Lookup(p.MainScene); {
	case p.MainScene.IsZero():
		out = append(out, diag(CodeStartScene, problems.Error, "project.aego names no mainScene", project.FileName))
	case !ok || e.Missing || e.Kind != asset.ArtScene:
		out = append(out, diag(CodeStartScene, problems.Error, "the start scene "+p.MainScene.String()+" is not an imported scene", project.FileName))
	}
	sev := problems.Warning
	if profile == bundle.Release {
		sev = problems.Error
	}
	s.idx.Each(func(e *asset.Entry) {
		if e.Missing || s.exportSkip(e) {
			return
		}
		for _, d := range e.Deps {
			if !resolves(s.idx, d) {
				out = append(out, diag(CodeMissing, sev, e.Source+" references a missing asset "+d.String(), e.Source))
			}
		}
	})
	if p.Icon != "" {
		if _, err := icon.Load(filepath.Join(p.Root, filepath.FromSlash(p.Icon))); err != nil {
			out = append(out, diag(CodeIcon, problems.Error, err.Error(), project.FileName))
		}
	}
	return append(out, validate(s, partial)...)
}

func validate(s *Snapshot, partial bool) []problems.Diagnostic {
	internal := func(err error) []problems.Diagnostic {
		return []problems.Diagnostic{diag(CodeInternal, problems.Error, "cannot run the asset checks: "+err.Error(), "")}
	}
	types, err := assembly.NewTypes()
	if err != nil {
		return internal(err)
	}
	bindings := bind.New()
	man, err := builtin.Runtime().Load(types, bindings, assembly.Options{})
	if err != nil {
		return internal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		return internal(err)
	}
	if err := publishScriptSchemas(s, types.Schema(), bindings.Surface()); err != nil {
		return []problems.Diagnostic{diag(CodeScripts, problems.Error, "cannot load the compiled script schemas: "+firstLine(err.Error()), "")}
	}
	out := man.ValidateAssets(types.Schema(), s, partial, nil)
	res := &resolver{s: s}
	checkedParticles := make(map[ids.AssetID]bool)
	s.idx.Each(func(e *asset.Entry) {
		if e.Missing || e.Error != "" || (e.Kind != asset.ArtScene && e.Kind != asset.ArtPrefab && (e.Kind != asset.ArtPreset || s.exportSkip(e))) {
			return
		}
		src, ok := res.blob(e.ID, e.Kind)
		if !ok {
			out = append(out, diag(CodeScene, problems.Error, e.Source+" has no "+string(e.Kind)+" artifact", e.Source))
			return
		}
		if e.Kind == asset.ArtPreset {
			if _, err := particles.ReadPreset(types, res, src); err != nil {
				out = append(out, diag(CodeScene, problems.Error, e.Source+": "+firstLine(err.Error()), e.Source))
				return
			}
		}
		sc, rep, err := sceneio.Load(src, e.Source, types, res, sceneio.LoadOptions{KeepIDs: true, CheckLiterals: true})
		if err != nil {
			out = append(out, diag(CodeScene, problems.Error, e.Source+" cannot be loaded: "+firstLine(err.Error()), e.Source))
			return
		}
		if s.current != nil {
			out = append(out, res.checkParticleInputs(sc, e.Source, checkedParticles)...)
		}
		for _, d := range module.ValidateScene(sc, nil) {
			if d.Subject.File == "" {
				d.Subject.File = e.Source
			}
			out = append(out, d)
		}
		if partial {
			return
		}
		for _, u := range rep.Unknown {
			out = append(out, diag(CodeUnknown, problems.Warning, e.Source+" uses "+u+", which no module registers; it is kept but does nothing", e.Source))
		}
	})
	return out
}

type resolver struct {
	s *Snapshot
}

func (r *resolver) blob(id ids.AssetID, kind asset.ArtifactKind) ([]byte, bool) {
	e, ok := r.s.idx.Lookup(id)
	if !ok || e.Missing || e.Kind != kind {
		return nil, false
	}
	a, ok := e.Artifact(kind)
	if !ok {
		return nil, false
	}
	data, err := r.s.files.ReadFile(a.Path)
	return data, err == nil
}

func (r *resolver) Texture(ids.AssetID) (texture.Region, bool) {
	return texture.Region{}, false
}

func (r *resolver) Mesh(ids.AssetID) (*mesh.Mesh, bool) {
	return nil, false
}

func (r *resolver) Scene(id ids.AssetID) ([]byte, bool) {
	return r.blob(id, asset.ArtScene)
}

func (r *resolver) Prefab(id ids.AssetID) ([]byte, bool) {
	return r.blob(id, asset.ArtPrefab)
}

func (r *resolver) Generation() uint32 {
	return 1
}

func (r *resolver) Subscribe(func(asset.Reloaded)) func() {
	return func() {}
}

var _ asset.Resolver = (*resolver)(nil)
