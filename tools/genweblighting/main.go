//go:build ignore

// genweblighting prepares the shared GPU integration fixture. Browser/player
// code embeds AESH only and never imports shaderx or expands source includes.
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"aego/asset"
	"aego/engine/res"
	"aego/render/shaderx"
)

func main() {
	files, err := filepath.Glob("tests/lightinggolden/fixtures/*.aeshader")
	if err != nil {
		panic(err)
	}
	for _, file := range files {
		src, err := os.ReadFile(file)
		if err != nil {
			panic(err)
		}
		u, diags := shaderx.Parse(src, file, func(path string) (string, []byte, error) {
			p := "shaders/include/" + strings.TrimPrefix(path, "engine/")
			data, err := fs.ReadFile(res.FS(), p)
			return "engine/" + p, data, err
		})
		if len(diags) > 0 {
			panic(fmt.Sprintf("%s: %v", file, diags))
		}
		data := asset.EncodeShader(asset.BuildShader(u))
		if err = os.WriteFile(strings.TrimSuffix(file, ".aeshader")+".aesh", data, 0644); err != nil {
			panic(err)
		}
	}
}
