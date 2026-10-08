package templates

import (
	"aego/asset"
	"aego/core/ids"
	"aego/editor/context"
)

func ForAsset(ctx *context.Editor, id ids.AssetID, kind asset.ArtifactKind, name string) (Template, bool) {
	switch {
	case kind == asset.ArtMesh:
		return MeshFor(id, kind, name), true
	case context.AssetMatches("texture", kind):
		return SpriteWith(id, name, SpriteSize(ctx, id)), true
	}
	return Template{}, false
}
