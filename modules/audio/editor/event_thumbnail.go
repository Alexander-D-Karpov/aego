package editor

import (
	"aego/asset"
	"aego/core/ids"
	"aego/editor/core/preview"
	"aego/editor/ext"
	"aego/format/aetext"
	"fmt"
	"image"
	"path/filepath"
	"strings"
)

func registerEventThumbnails(r *ext.Registry) error {
	return r.Thumbnail(asset.ArtData, preview.Handler{Version: "audio-event.v1", ReadSource: true, Build: eventCard})
}
func eventCard(c *preview.Card) (bool, error) {
	doc, err := aetext.Parse(c.Source, c.Entry.Source)
	if err != nil {
		return false, nil
	}
	typ, _ := doc.Attr("type")
	if typ.Str != "audio.AudioEvent" {
		return false, nil
	}
	v, _ := doc.Field("variants")
	c.Label(4, 2, fmt.Sprintf("EVENT / %d clips", len(v.List)), true)
	for i, ref := range v.List[:min(3, len(v.List))] {
		id, err := ids.ParseAssetID(ref.Str)
		if err != nil {
			return true, err
		}
		y := 16 + i*36
		name, err := c.Blit(id, image.Rect(3, y+13, 125, y+33))
		if err != nil {
			return true, err
		}
		c.Label(4, y, strings.TrimSuffix(filepath.Base(name), filepath.Ext(name)), false)
	}
	return true, nil
}
