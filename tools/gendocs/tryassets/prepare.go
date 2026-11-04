// Package tryassets prepares documentation resources at build time. Browser
// sessions decode AETB and use the ordinary loader; they never import sources.
package tryassets

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"aego/asset"
	"aego/asset/importers/atlas"
	matimp "aego/asset/importers/material"
	shaderimp "aego/asset/importers/shader"
	teximp "aego/asset/importers/texture"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/log"
	"aego/project"
	"aego/render/shader"
	"aego/render/shaderx"
)

// Prepare imports a bounded source project copied to an isolated temporary
// directory. Existing .meta IDs are mandatory and the source tree is read-only.
func Prepare(source string) ([]byte, error) {
	root, err := filepath.Abs(source)
	if err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp("", "aego-try-assets-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	hash := sha256.New()
	fmt.Fprintf(hash, "AETB=%d;pipeline=%d;texture=%d;atlas=%d;material=%d;shader=%d;include=%d;generator=%d;abi=%d\n", asset.TryArtifactsVersion, pipeline.PipelineVersion, teximp.Version, atlas.GroupVersion, matimp.Version, shaderimp.Version, shaderimp.IncludeVersion, shaderx.GeneratorVersion, shader.ABI)
	count, total := 0, 0
	err = filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("try source cannot contain symlinks: %s", rel)
		}
		if entry.IsDir() {
			if rel != "." && strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(tmp, rel), 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		count++
		if !info.Mode().IsRegular() || count > 256 || info.Size() > asset.MaxTryArtifactBytes {
			return fmt.Errorf("try source file budget exceeded: %s", rel)
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		total += len(data)
		if total > asset.MaxTryArtifactsBytes {
			return fmt.Errorf("try source byte budget exceeded")
		}
		path := filepath.ToSlash(rel)
		var lengths [16]byte
		binary.LittleEndian.PutUint64(lengths[:8], uint64(len(path)))
		binary.LittleEndian.PutUint64(lengths[8:], uint64(len(data)))
		hash.Write(lengths[:])
		hash.Write([]byte(path))
		hash.Write(data)
		return os.WriteFile(filepath.Join(tmp, rel), data, 0o644)
	})
	if err != nil {
		return nil, err
	}
	p, err := project.Load(tmp)
	if err != nil {
		return nil, err
	}
	imps := pipeline.NewImporterRegistry()
	for _, imp := range []pipeline.Importer{atlas.Wrap(teximp.New()), matimp.New(), shaderimp.New(), shaderimp.NewInclude()} {
		if err := imps.Register(imp); err != nil {
			return nil, err
		}
	}
	pl, err := pipeline.Open(p, imps, log.Nop())
	if err != nil {
		return nil, err
	}
	defer pl.Close()
	pl.SetMetaMode(pipeline.MetaReadOnly)
	if _, _, err := pl.Scan(); err != nil {
		return nil, err
	}
	if issues, _ := pl.MetaIssues(); len(issues) > 0 {
		return nil, fmt.Errorf("try source metadata must be complete before preparation: %s", issues[0].Message())
	}
	batch, err := pl.ImportAll()
	if err != nil {
		return nil, err
	}
	if len(batch.Failed) > 0 {
		var messages []string
		for id, err := range batch.Failed {
			messages = append(messages, fmt.Sprintf("%s: %v", id, err))
		}
		slices.Sort(messages)
		return nil, fmt.Errorf("try source import failed: %s", strings.Join(messages, "; "))
	}
	bundle := asset.TryArtifacts{}
	var buildErr error
	builtins := map[ids.AssetID]bool{}
	pl.Index().Each(func(entry *asset.Entry) {
		if buildErr != nil || entry.Kind == asset.ArtShaderInclude {
			return
		}
		if entry.Missing || entry.Error != "" {
			buildErr = fmt.Errorf("try asset %s is unavailable: %s", entry.ID, entry.Error)
			return
		}
		out := asset.TryAsset{ID: entry.ID, Kind: entry.Kind, Name: entry.Name}
		for _, dep := range entry.Deps {
			if e, ok := pl.Index().Lookup(dep); ok && e.Kind == asset.ArtShaderInclude {
				continue
			}
			out.Deps = append(out.Deps, dep)
			if asset.IsBuiltin(dep) {
				builtins[dep] = true
			}
		}
		for _, artifact := range entry.Artifacts {
			data, err := pl.Artifacts().ReadFile(artifact.Path)
			if err != nil {
				buildErr = err
				return
			}
			out.Artifacts = append(out.Artifacts, asset.TryArtifact{Kind: artifact.Kind, Name: artifact.Name, Data: data})
		}
		bundle.Assets = append(bundle.Assets, out)
	})
	if buildErr != nil {
		return nil, buildErr
	}
	var builtinIDs []ids.AssetID
	for id := range builtins {
		builtinIDs = append(builtinIDs, id)
	}
	slices.SortFunc(builtinIDs, func(a, b ids.AssetID) int { return strings.Compare(a.String(), b.String()) })
	for _, id := range builtinIDs {
		hash.Write([]byte(id.String()))
		if data, ok := asset.BuiltinShader(id); ok {
			hash.Write(data)
		}
		if data, ok := asset.BuiltinMaterial(id); ok {
			hash.Write(data)
		}
	}
	copy(bundle.SourceHash[:], hash.Sum(nil))
	return asset.EncodeTryArtifacts(&bundle)
}

// Generate writes or verifies X.aetry next to X.assets. A fresh import in check
// mode detects source/meta, importer, ABI and embedded shader changes.
func Generate(source string, check bool) (string, error) {
	if !strings.HasSuffix(source, ".assets") {
		return "", fmt.Errorf("try source project must end in .assets")
	}
	data, err := Prepare(source)
	if err != nil {
		return "", err
	}
	target := strings.TrimSuffix(source, ".assets") + ".aetry"
	if check {
		existing, err := os.ReadFile(target)
		if err != nil {
			return "", err
		}
		if !bytes.Equal(existing, data) {
			return "", fmt.Errorf("%s is stale; run gendocs -prepare-try %s", target, source)
		}
		return target, nil
	}
	return target, os.WriteFile(target, data, 0o644)
}
