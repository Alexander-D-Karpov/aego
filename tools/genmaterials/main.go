// genmaterials prepares the engine's ordinary .aeshader/.aemat assets using the
// same importers as project content. Only the generated artifacts ship at runtime.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"aego/asset"
	matimp "aego/asset/importers/material"
	shaderimp "aego/asset/importers/shader"
	"aego/asset/pipeline"
	"aego/core/ids"
)

func main() {
	root := flag.String("root", ".", "engine repository")
	check := flag.Bool("check", false, "check committed artifacts without changing them")
	flag.Parse()
	if err := generate(*root, *check); err != nil {
		fmt.Fprintln(os.Stderr, "genmaterials:", err)
		os.Exit(1)
	}
}

func generate(root string, check bool) error {
	shaders := shaderimp.New()
	shaders.Require = true
	materials := matimp.New()
	read := func(path string) ([]byte, error) {
		rel, ok := strings.CutPrefix(path, "engine://")
		if !ok {
			return nil, fmt.Errorf("builtin include outside engine resources: %s", path)
		}
		return os.ReadFile(filepath.Join(root, "engine", "res", filepath.FromSlash(rel)))
	}
	stale := 0
	for _, name := range asset.BuiltinMaterialNames() {
		base := strings.ToLower(name)
		shaderID, _ := asset.BuiltinShaderNamed(name)
		materialID, _ := asset.BuiltinMaterialNamed(name)
		jobs := []struct {
			ext, out string
			id       ids.AssetID
			importer pipeline.Importer
		}{
			{".aeshader", ".aesh", shaderID, shaders},
			{".aemat", ".aemt", materialID, materials},
		}
		for _, job := range jobs {
			path := "engine://shaders/materials/" + base + job.ext
			src, err := read(path)
			if err != nil {
				return err
			}
			result, err := job.importer.Import(&pipeline.ImportContext{ID: job.id, Path: path, Source: src, Read: read})
			if err != nil {
				return err
			}
			if len(result.Artifacts) != 1 {
				return fmt.Errorf("%s produced %d artifacts", path, len(result.Artifacts))
			}
			data := result.Artifacts[0].Data
			dst := filepath.Join(root, "asset", "res", "materials", base+job.out)
			old, err := os.ReadFile(dst)
			if err == nil && bytes.Equal(old, data) {
				continue
			}
			if check {
				fmt.Fprintln(os.Stderr, "out of date:", dst)
				stale++
				continue
			}
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(dst, data, 0o644); err != nil {
				return err
			}
			fmt.Println("wrote", dst)
		}
	}
	if stale > 0 {
		return fmt.Errorf("%d builtin artifacts need make materials", stale)
	}
	return nil
}
