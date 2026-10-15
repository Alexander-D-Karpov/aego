package assetops

import (
	"image"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/errs"
	"aego/core/ids"
	"aego/editor/core/context"
	"aego/platform/clip"
)

const PastedName = "Pasted"

func Report(ctx *context.Editor, err error) {
	if err != nil {
		ctx.Notify(context.LevelError, err.Error())
	}
}

func NextNumbered(ctx *context.Editor, dir, base, sep, ext string) string {
	for i := 1; ; i++ {
		if rel := path.Join(dir, base+sep+strconv.Itoa(i)+ext); !Exists(ctx, rel) {
			return rel
		}
	}
}

func EnsureDir(ctx *context.Editor, dir string) error {
	if dir == "" || dir == "." || Exists(ctx, dir) {
		return nil
	}
	if err := EnsureDir(ctx, path.Dir(dir)); err != nil {
		return err
	}
	return ctx.Pipeline.Mkdir(dir)
}

func CreatePNG(ctx *context.Editor, rel string, img image.Image) (ids.AssetID, error) {
	data := clip.EncodePNG(img)
	if len(data) == 0 {
		return ids.AssetID{}, errs.New(errs.EditorDocument, "cannot encode the image as PNG")
	}
	return CreateFile(ctx, rel, data)
}

func RelIn(pl *pipeline.Pipeline, abs string) (string, bool) {
	if pl == nil {
		return "", false
	}
	rel, err := filepath.Rel(pl.Abs("."), filepath.Clean(abs))
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return rel, true
}

func ProjectRel(ctx *context.Editor, abs string) (string, bool) {
	return RelIn(ctx.Pipeline, abs)
}

func sourcePNG(ctx *context.Editor, id ids.AssetID) ([]byte, bool) {
	e, ok := ctx.Pipeline.Entry(id)
	if !ok || e.Kind != asset.ArtTexture {
		return nil, false
	}
	rel, ok := ctx.Pipeline.PathOf(id)
	if !ok {
		return nil, false
	}
	data, err := os.ReadFile(ctx.Pipeline.Abs(rel))
	if err != nil {
		return nil, false
	}
	if strings.EqualFold(path.Ext(rel), ".png") {
		return data, true
	}
	img, err := clip.DecodeImage(data)
	if err != nil {
		return nil, false
	}
	out := clip.EncodePNG(img)
	return out, len(out) > 0
}

func AbsPaths(ctx *context.Editor, list []ids.AssetID) []string {
	var out []string
	for _, id := range list {
		if rel, ok := ctx.Pipeline.PathOf(id); ok {
			out = append(out, filepath.Clean(ctx.Pipeline.Abs(rel)))
		}
	}
	return out
}

func CopyAssets(ctx *context.Editor, list []ids.AssetID) []string {
	if ctx.Clipboard == nil || ctx.Pipeline == nil {
		return nil
	}
	paths := AbsPaths(ctx, list)
	if len(paths) == 0 {
		return nil
	}
	if len(list) == 1 {
		if data, ok := sourcePNG(ctx, list[0]); ok && clip.SetImagePNG(ctx.Clipboard, data, paths) {
			return paths
		}
	}
	if !clip.SetFiles(ctx.Clipboard, paths) {
		return nil
	}
	return paths
}

func CanPaste(ctx *context.Editor) bool {
	if ctx.Clipboard == nil || ctx.Pipeline == nil {
		return false
	}
	if clip.HasFiles(ctx.Clipboard) || clip.HasImage(ctx.Clipboard) {
		return true
	}
	_, ok := clip.Files(ctx.Clipboard)
	return ok
}

func SamePaths(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	norm := func(in []string) []string {
		out := make([]string, len(in))
		for i, s := range in {
			out[i] = filepath.Clean(s)
		}
		slices.Sort(out)
		return out
	}
	return slices.Equal(norm(a), norm(b))
}
