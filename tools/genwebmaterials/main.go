//go:build ignore

// Prepare test shader sources with the native importer model. The wasm test
// itself embeds AESH only and never imports shaderx or launches a validator.
package main

import (
	"aego/engine/res"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"aego/asset"
	"aego/render/shaderx"
)

func main() {
	files, err := filepath.Glob("tests/webmaterials/fixtures/*.aeshader")
	if err != nil {
		panic(err)
	}
	for _, file := range files {
		src, err := os.ReadFile(file)
		if err != nil {
			panic(err)
		}
		u, diags := shaderx.Parse(src, filepath.Base(file), func(name string) (string, []byte, error) {
			path := "shaders/include/" + strings.TrimPrefix(name, "engine/")
			data, err := fs.ReadFile(res.FS(), path)
			return "engine/" + path, data, err
		})
		if len(diags) > 0 {
			panic(diags)
		}
		data := asset.EncodeShader(asset.BuildShader(u))
		dst := file[:len(file)-len(".aeshader")] + ".aesh"
		if err := os.WriteFile(dst, data, 0644); err != nil {
			panic(err)
		}
		fmt.Println(dst)
	}
}
