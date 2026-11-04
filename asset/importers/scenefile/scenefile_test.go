package scenefile

import (
	"fmt"
	"testing"

	"aego/asset/pipeline"
	"aego/core/ids"
)

func TestNullAssetReferenceIsNotAMissingDependency(t *testing.T) {
	for _, imp := range []*Importer{NewScene(), NewPrefab()} {
		t.Run(imp.ID(), func(t *testing.T) {
			source := fmt.Sprintf(`%s version=1 {
  node "10000000000000000000000000000001" name="Mesh" {
    component "engine.Mesh2D" {
      texture=asset("00000000000000000000000000000000")
      material=asset("00000000000000000000000000000011")
      future=[asset("00000000000000000000000000000000"), asset("00000000000000000000000000000022")]
    }
  }
}`, imp.heads[0])
			result, err := imp.Import(&pipeline.ImportContext{Path: "test" + imp.ext, Source: []byte(source)})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Deps) != 2 || result.Deps[0] != (ids.AssetID{Lo: 0x11}) || result.Deps[1] != (ids.AssetID{Lo: 0x22}) {
				t.Fatalf("null dependency retained or unresolved nonzero ID lost: %v", result.Deps)
			}
		})
	}
}
