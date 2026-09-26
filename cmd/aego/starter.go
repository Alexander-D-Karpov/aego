package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"

	texImporter "aego/asset/importers/texture"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/format/aetext"
	"aego/project"
	"aego/scene/sceneio"
)

func placeholderPNG() ([]byte, error) {
	const size, cell = 32, 8
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	light := color.NRGBA{R: 0xE0, G: 0x7B, B: 0x39, A: 0xFF}
	dark := color.NRGBA{R: 0x20, G: 0x24, B: 0x2C, A: 0xFF}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			c := dark
			if ((x/cell)+(y/cell))%2 == 0 {
				c = light
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writePlaceholder(p *project.Project) (ids.AssetID, error) {
	data, err := placeholderPNG()
	if err != nil {
		return ids.AssetID{}, err
	}
	path := filepath.Join(p.AssetsDir(), "placeholder.png")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return ids.AssetID{}, err
	}
	imp := texImporter.New()
	meta := pipeline.NewMeta(imp.ID(), imp.Version(), imp.Defaults())
	if err := os.WriteFile(path+pipeline.MetaSuffix, pipeline.WriteMeta(meta), 0o644); err != nil {
		return ids.AssetID{}, err
	}
	return meta.ID, nil
}

func writeStarterScene(p *project.Project) error {
	spriteID, err := writePlaceholder(p)
	if err != nil {
		return err
	}
	doc := &aetext.Node{Head: "scene"}
	doc.Attrs = append(doc.Attrs, aetext.Field{Name: "version", Value: aetext.Int(sceneio.FormatVersion)})
	doc.Blocks = append(doc.Blocks, cameraNode(), spriteNode(spriteID))

	scenePath := filepath.Join(p.ScenesDir(), "main.aescene")
	if err := os.WriteFile(scenePath, aetext.Write(doc), 0o644); err != nil {
		return err
	}
	meta := pipeline.NewMeta("scene", 1, nil)
	if err := os.WriteFile(scenePath+pipeline.MetaSuffix, pipeline.WriteMeta(meta), 0o644); err != nil {
		return err
	}
	p.MainScene = meta.ID
	return p.Save()
}

func nodeBlock(name string) *aetext.Node {
	n := &aetext.Node{Head: "node", Label: ids.NewNodeID().String()}
	n.Attrs = append(n.Attrs,
		aetext.Field{Name: "name", Value: aetext.String(name)},
		aetext.Field{Name: "enabled", Value: aetext.Bool(true)},
		aetext.Field{Name: "layers", Value: aetext.Int(1)},
	)
	n.Blocks = append(n.Blocks, transformBlock(0, 0))
	return n
}

func cameraNode() *aetext.Node {
	n := nodeBlock("Camera")
	c := &aetext.Node{Head: "component", Label: "engine.Camera"}
	c.Attrs = append(c.Attrs, aetext.Field{Name: "version", Value: aetext.Int(1)})
	c.Fields = append(c.Fields,
		aetext.Field{Name: "zoom", Value: aetext.Float32(64)},
		aetext.Field{Name: "viewport", Value: aetext.List(aetext.Float32(0), aetext.Float32(0), aetext.Float32(1), aetext.Float32(1))},
		aetext.Field{Name: "mask", Value: aetext.Int(-1)},
		aetext.Field{Name: "order", Value: aetext.Int(0)},
		aetext.Field{Name: "clear", Value: aetext.Bool(true)},
		aetext.Field{Name: "clearColor", Value: aetext.Color(0x0a0d12ff)},
		aetext.Field{Name: "scale", Value: aetext.Ident("free")},
		aetext.Field{Name: "virtual", Value: aetext.List(aetext.Float32(0), aetext.Float32(0))},
		aetext.Field{Name: "active", Value: aetext.Bool(true)},
	)
	n.Blocks = append(n.Blocks, c)
	return n
}

func spriteNode(sprite ids.AssetID) *aetext.Node {
	n := nodeBlock("Sprite")
	c := &aetext.Node{Head: "component", Label: "engine.SpriteRenderer"}
	c.Attrs = append(c.Attrs, aetext.Field{Name: "version", Value: aetext.Int(1)})
	c.Fields = append(c.Fields,
		aetext.Field{Name: "sprite", Value: aetext.AssetRef(sprite.String())},
		aetext.Field{Name: "layer", Value: aetext.Int(0)},
		aetext.Field{Name: "order", Value: aetext.Int(0)},
		aetext.Field{Name: "tint", Value: aetext.Color(0xffffffff)},
		aetext.Field{Name: "origin", Value: aetext.List(aetext.Float32(0.5), aetext.Float32(0.5))},
		aetext.Field{Name: "size", Value: aetext.List(aetext.Float32(2), aetext.Float32(2))},
		aetext.Field{Name: "flipX", Value: aetext.Bool(false)},
		aetext.Field{Name: "flipY", Value: aetext.Bool(false)},
		aetext.Field{Name: "blend", Value: aetext.Ident("premultiplied")},
		aetext.Field{Name: "filter", Value: aetext.Ident("nearest")},
	)
	n.Blocks = append(n.Blocks, c)
	return n
}
