//go:build aego_golden

package golden

import (
	"testing"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/services"
	"aego/editor/panels/scenes"
	"aego/project"
)

func TestScenesPanelGolden(t *testing.T) {
	env := newEditorEnv(t)
	list := []pipeline.AssetInfo{
		{ID: ids.AssetID{Hi: 1, Lo: 1}, Kind: asset.ArtScene, Path: "scenes/main.aescene"},
		{ID: ids.AssetID{Hi: 1, Lo: 2}, Kind: asset.ArtScene, Path: "scenes/levels/forest.aescene"},
		{ID: ids.AssetID{Hi: 1, Lo: 3}, Kind: asset.ArtScene, Path: "scenes/levels/cave.aescene"},
	}
	p := scenes.New(scenes.Deps{
		Assets:  func() []pipeline.AssetInfo { return list },
		Project: &project.Project{MainScene: list[0].ID},
	})
	img, err := env.draw(func(full mathx.Rect) { p.Draw(&services.Frame{UI: env.ui}, full) })
	checkGolden(t, "editor_scenes", img, err)
}
