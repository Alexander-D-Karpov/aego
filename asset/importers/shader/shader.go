package shader

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/render/shaderx"
)

const (
	Version        = 1
	IncludeVersion = 1
	RequireEnv     = "AEGO_REQUIRE_GLSLANG"
	engineInclude  = "engine://shaders/include/"
	projectInclude = "assets/shaders/include/"
)

type Importer struct {
	Validator string
	Require   bool
	mu        sync.Mutex
	diags     map[string][]shaderx.Diag
	rev       uint64
}

func New() *Importer {
	return &Importer{Validator: "glslangValidator", Require: os.Getenv(RequireEnv) == "1", diags: map[string][]shaderx.Diag{}}
}

// Diagnostics are the SHD problems of the last import of every shader, for the Problems panel.
func (i *Importer) Diagnostics() ([]shaderx.Diag, uint64) {
	i.mu.Lock()
	defer i.mu.Unlock()
	var out []shaderx.Diag
	for _, p := range slices.Sorted(maps.Keys(i.diags)) {
		out = append(out, i.diags[p]...)
	}
	return out, i.rev
}

func (i *Importer) record(path string, diags []shaderx.Diag) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if len(diags) == 0 && len(i.diags[path]) == 0 {
		return
	}
	if len(diags) == 0 {
		delete(i.diags, path)
	} else {
		i.diags[path] = diags
	}
	i.rev++
}

func (*Importer) ID() string                  { return "shader" }
func (*Importer) Version() int                { return Version }
func (*Importer) Extensions() []string        { return []string{".aeshader"} }
func (*Importer) Defaults() pipeline.Settings { return pipeline.Settings{} }

func includePath(p string) (string, error) {
	clean := path.Clean(p)
	if clean != p || strings.HasPrefix(clean, "../") || path.IsAbs(clean) || path.Ext(clean) != ".glsl" {
		return "", errors.New("an include is a relative .glsl path that does not leave its root")
	}
	if rest, ok := strings.CutPrefix(clean, "engine/"); ok {
		return engineInclude + rest, nil
	}
	return projectInclude + clean, nil
}

func (i *Importer) Import(ctx *pipeline.ImportContext) (*pipeline.Result, error) {
	var deps []ids.AssetID
	inc := func(p string) (string, []byte, error) {
		rel, err := includePath(p)
		if err != nil {
			return "", nil, err
		}
		if ctx.Read == nil {
			return "", nil, fmt.Errorf("includes cannot be read here")
		}
		src, err := ctx.Read(rel)
		if err != nil {
			return "", nil, err
		}
		if ctx.Lookup != nil {
			if id, ok := ctx.Lookup(rel); ok && !slices.Contains(deps, id) {
				deps = append(deps, id)
			}
		}
		return rel, src, nil
	}
	u, diags := shaderx.Parse(ctx.Source, ctx.Path, inc)
	if len(diags) == 0 {
		diags = i.validate(ctx, u)
	}
	i.record(ctx.Path, diags)
	if len(diags) > 0 {
		return nil, report(ctx.Path, diags)
	}
	return &pipeline.Result{
		Kind:      asset.ArtShader,
		Deps:      deps,
		Artifacts: []pipeline.Produced{{Kind: asset.ArtShader, Data: asset.EncodeShader(asset.BuildShader(u))}},
	}, nil
}

type job struct {
	path string
	locs []shaderx.Loc
}

func (i *Importer) validate(ctx *pipeline.ImportContext, u *shaderx.Unit) []shaderx.Diag {
	bin, err := exec.LookPath(i.Validator)
	if err != nil {
		if i.Require {
			return []shaderx.Diag{{Code: shaderx.SHD001, File: ctx.Path, Msg: "glslangValidator is required here: install glslang-tools"}}
		}
		if ctx.Log != nil {
			ctx.Log.Warn("glslangValidator is not installed, only the driver checks this shader; install glslang-tools", log.F("path", ctx.Path))
		}
		return nil
	}
	dir, err := os.MkdirTemp("", "aego-shader-")
	if err != nil {
		return []shaderx.Diag{{Code: shaderx.SHD001, File: ctx.Path, Msg: err.Error()}}
	}
	defer os.RemoveAll(dir)
	var jobs []job
	var args []string
	for vi, mask := range u.Combos {
		for si := 0; si < 4; si++ {
			frag := si%2 == 1
			src, locs := u.Source(frag, si >= 2, mask)
			ext := ".vert"
			if frag {
				ext = ".frag"
			}
			p := filepath.Join(dir, fmt.Sprintf("v%d_%d%s", vi, si, ext))
			if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
				return []shaderx.Diag{{Code: shaderx.SHD001, File: ctx.Path, Msg: err.Error()}}
			}
			jobs = append(jobs, job{path: p, locs: locs})
			args = append(args, p)
		}
	}
	out, runErr := exec.Command(bin, args...).CombinedOutput()
	lines := strings.Split(string(out), "\n")
	var diags []shaderx.Diag
	for _, j := range jobs {
		var sb strings.Builder
		for _, l := range lines {
			if strings.Contains(l, j.path+":") {
				sb.WriteString(l)
				sb.WriteByte('\n')
			}
		}
		for _, d := range shaderx.MapLog(sb.String(), j.locs, u.Files, shaderx.SHD001) {
			if !slices.Contains(diags, d) {
				diags = append(diags, d)
			}
		}
	}
	if runErr != nil && len(diags) == 0 {
		diags = append(diags, shaderx.Diag{Code: shaderx.SHD001, File: ctx.Path, Msg: strings.TrimSpace(string(out))})
	}
	return diags
}

func report(p string, diags []shaderx.Diag) error {
	msgs := make([]string, len(diags))
	for i, d := range diags {
		msgs[i] = d.Error()
	}
	d := diags[0]
	return errs.New(errs.AssetImportFailed, d.Msg).
		With(log.F("path", p), log.F("code", d.Code), log.F("file", d.File), log.F("line", d.Line), log.F("diagnostics", strings.Join(msgs, "\n")))
}

type Include struct{}

func NewInclude() *Include { return &Include{} }

func (*Include) ID() string                  { return "shaderinclude" }
func (*Include) Version() int                { return IncludeVersion }
func (*Include) Extensions() []string        { return []string{".glsl"} }
func (*Include) Defaults() pipeline.Settings { return pipeline.Settings{} }

func (*Include) Import(ctx *pipeline.ImportContext) (*pipeline.Result, error) {
	return &pipeline.Result{
		Kind:      asset.ArtShaderInclude,
		Artifacts: []pipeline.Produced{{Kind: asset.ArtShaderInclude, Data: ctx.Source}},
	}, nil
}
