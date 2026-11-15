package shader

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
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
	"time"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	rshader "aego/render/shader"
	"aego/render/shaderx"
)

const (
	Version        = 3
	IncludeVersion = 1
	RequireEnv     = "AEGO_REQUIRE_GLSLANG"
	engineInclude  = "engine://shaders/include/"
	projectInclude = "assets/shaders/include/"
)

type Importer struct {
	Validator string
	Require   bool
	// DisableExternal prevents validator processes in untrusted remote projects.
	// Schema parsing still runs, but the result is never marked reference-checked.
	DisableExternal bool
	mu              sync.Mutex
	diags           map[string][]shaderx.Diag
	rev             uint64
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

// Revision allows consumers to check for changes without rebuilding the sorted diagnostics.
func (i *Importer) Revision() uint64 {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.rev
}

func (i *Importer) record(path string, diags []shaderx.Diag) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.diags == nil {
		i.diags = make(map[string][]shaderx.Diag)
	}
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

func includePath(owner, p string) (string, error) {
	if strings.ContainsAny(p, "\\\x00:") || path.IsAbs(p) || path.Ext(p) != ".glsl" {
		return "", errors.New("an include is a .glsl resource path, not an OS path or URL")
	}
	if strings.HasPrefix(p, "./") || strings.HasPrefix(p, "../") {
		root := "assets/"
		if strings.HasPrefix(owner, "engine://shaders/") {
			root = "engine://shaders/"
		}
		localOwner, ok := strings.CutPrefix(owner, root)
		if !ok || localOwner == "" || path.IsAbs(localOwner) || path.Clean(localOwner) != localOwner || strings.ContainsAny(localOwner, "\\\x00:") {
			return "", fmt.Errorf("relative include owner %q is outside the assets or engine shader namespace", owner)
		}
		local := path.Clean(path.Join(path.Dir(localOwner), p))
		if local == "." || local == ".." || strings.HasPrefix(local, "../") || path.IsAbs(local) {
			return "", fmt.Errorf("relative include %q leaves the %s namespace", p, root)
		}
		return root + local, nil
	}
	// Bare paths retain the original shared include-root contract. Explicit
	// relative paths never fall back to this namespace when their file is absent.
	clean := path.Clean(p)
	if clean != p || strings.HasPrefix(clean, "../") {
		return "", errors.New("an include is a relative .glsl path that does not leave its root")
	}
	if rest, ok := strings.CutPrefix(clean, "engine/"); ok {
		return engineInclude + rest, nil
	}
	return projectInclude + clean, nil
}

func parse(ctx *pipeline.ImportContext) (*shaderx.Unit, []ids.AssetID, []shaderx.Diag) {
	var deps []ids.AssetID
	inc := func(owner, p string) (string, []byte, error) {
		rel, err := includePath(owner, p)
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
	u, diags := shaderx.ParseWithIncludes(ctx.Source, ctx.Path, inc)
	return u, deps, diags
}

// CacheSalt hashes transitive sources, including engine:// files without AssetIDs, before artifact reuse.
// Changes in validator availability/version invalidate an unchecked cache.
func (i *Importer) CacheSalt(ctx *pipeline.ImportContext) ([]byte, error) {
	u, _, diags := parse(ctx)
	if len(diags) > 0 {
		for n := range diags {
			diags[n].AssetID = ctx.ID.String()
		}
		i.record(ctx.Path, diags)
		return nil, report(ctx.Path, diags)
	}
	h := sha256.New()
	h.Write([]byte("aego-shader;glsl330;essl300;"))
	var version [12]byte
	binary.LittleEndian.PutUint32(version[:4], Version)
	binary.LittleEndian.PutUint32(version[4:8], rshader.ABI)
	binary.LittleEndian.PutUint32(version[8:], shaderx.GeneratorVersion)
	h.Write(version[:])
	for _, d := range u.Dependencies {
		h.Write([]byte(d.Path))
		h.Write([]byte{0})
		h.Write(d.Hash[:])
	}
	if i.Require {
		h.Write([]byte("required"))
	}
	if i.DisableExternal {
		if i.Require {
			return nil, fmt.Errorf("shader reference validation is required but external tools are disabled")
		}
		h.Write([]byte("external-tools-disabled"))
		return h.Sum(nil), nil
	}
	bin, err := exec.LookPath(i.Validator)
	if err != nil {
		h.Write([]byte("unavailable"))
		if i.Require {
			ds := []shaderx.Diag{{Code: shaderx.SHD001, File: ctx.Path, AssetID: ctx.ID.String(), Msg: "glslangValidator is required here: install the pinned reference validator"}}
			i.record(ctx.Path, ds)
			return nil, report(ctx.Path, ds)
		}
	} else {
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		out, err := exec.CommandContext(c, bin, "--version").CombinedOutput()
		cancel()
		if err != nil {
			return nil, fmt.Errorf("shader validator version: %w: %s", err, out)
		}
		h.Write(out)
	}
	return h.Sum(nil), nil
}

func (i *Importer) Import(ctx *pipeline.ImportContext) (*pipeline.Result, error) {
	u, deps, diags := parse(ctx)
	checked := false
	if len(diags) == 0 {
		diags, checked = i.validate(ctx, u)
	}
	for n := range diags {
		diags[n].AssetID = ctx.ID.String()
	}
	i.record(ctx.Path, diags)
	if len(diags) > 0 {
		return nil, report(ctx.Path, diags)
	}
	a := asset.BuildShader(u)
	if checked {
		a.Validation |= rshader.ReferenceChecked
	}
	return &pipeline.Result{Kind: asset.ArtShader, Deps: deps, Artifacts: []pipeline.Produced{{Kind: asset.ArtShader, Data: asset.EncodeShader(a)}}}, nil
}

type job struct {
	path  string
	locs  []shaderx.Loc
	stage string
}

func (i *Importer) validate(ctx *pipeline.ImportContext, u *shaderx.Unit) ([]shaderx.Diag, bool) {
	if i.DisableExternal {
		if i.Require {
			return []shaderx.Diag{{Code: shaderx.SHD001, File: ctx.Path, Msg: "shader reference validation is required but external tools are disabled"}}, false
		}
		if ctx.Log != nil {
			ctx.Log.Warn("external shader validation is disabled; shader schema checked, reference and GPU linking are unverified", log.F("path", ctx.Path))
		}
		return nil, false
	}
	bin, err := exec.LookPath(i.Validator)
	if err != nil {
		if i.Require {
			return []shaderx.Diag{{Code: shaderx.SHD001, File: ctx.Path, Msg: "glslangValidator is required here: install the pinned reference validator"}}, false
		}
		if ctx.Log != nil {
			ctx.Log.Warn("glslangValidator is not installed; shader schema checked, reference and GPU linking are unverified", log.F("path", ctx.Path))
		}
		return nil, false
	}
	dir, err := os.MkdirTemp("", "aego-shader-")
	if err != nil {
		return []shaderx.Diag{{Code: shaderx.SHD001, File: ctx.Path, Msg: err.Error()}}, false
	}
	defer os.RemoveAll(dir)
	var diags []shaderx.Diag
	for vi, mask := range u.Combos {
		for backend := 0; backend < 2; backend++ {
			jobs := make([]job, 0, 2)
			args := []string{"-l"}
			for stage := 0; stage < 2; stage++ {
				src, locs := u.Source(stage == 1, backend == 1, mask)
				ext, stageName := ".vert", "vertex"
				if stage == 1 {
					ext, stageName = ".frag", "fragment"
				}
				name := filepath.Join(dir, fmt.Sprintf("v%d_%d%s", vi, backend, ext))
				if err := os.WriteFile(name, []byte(src), 0o644); err != nil {
					return []shaderx.Diag{{Code: shaderx.SHD001, File: ctx.Path, Msg: err.Error()}}, false
				}
				jobs = append(jobs, job{path: name, locs: locs, stage: stageName})
				args = append(args, name)
			}
			c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			out, runErr := exec.CommandContext(c, bin, args...).CombinedOutput()
			cancel()
			if runErr == nil {
				continue
			}
			backendName := "GLSL330"
			if backend == 1 {
				backendName = "ESSL300"
			}
			mapped := false
			for _, j := range jobs {
				var sb strings.Builder
				for _, line := range strings.Split(string(out), "\n") {
					if strings.Contains(line, j.path+":") {
						sb.WriteString(line)
						sb.WriteByte('\n')
					}
				}
				for _, d := range shaderx.MapLog(sb.String(), j.locs, u.Files, shaderx.SHD001) {
					d.Backend, d.Stage, d.Variant, d.RawLog = backendName, j.stage, mask, string(out)
					diags = append(diags, d)
					mapped = true
				}
			}
			if !mapped {
				msg := strings.TrimSpace(string(out))
				if msg == "" {
					msg = runErr.Error()
				}
				diags = append(diags, shaderx.Diag{Code: shaderx.SHD001, File: ctx.Path, Msg: msg, Backend: backendName, Stage: "link", Variant: mask, RawLog: string(out)})
			}
		}
	}
	return diags, len(diags) == 0
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
