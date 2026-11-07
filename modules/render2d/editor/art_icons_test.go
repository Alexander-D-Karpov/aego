package editor

import (
	"encoding/json"
	"image/png"
	"io/fs"
	"testing"

	"aego/engine/res"
)

// Check the embedded images, not just the SVG source or the icon-use list: a
// missing or blank baked region otherwise silently becomes a letter fallback.
func TestArtIconsAreBakedAtBothScales(t *testing.T) {
	ids := []string{
		"file.art", "panel.art_tools", "panel.art_timeline", "ui.hidden",
		"tool.pencil", "tool.eraser", "tool.fill", "tool.line", "tool.rectangle",
		"tool.ellipse", "tool.gradient", "tool.selection", "tool.wand", "tool.eyedropper",
	}
	data, err := fs.ReadFile(res.FS(), "icons/icons.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Atlases []struct {
			Scale int    `json:"scale"`
			File  string `json:"file"`
			Icons []struct {
				ID   string `json:"id"`
				Size int    `json:"size"`
				X    int    `json:"x"`
				Y    int    `json:"y"`
				W    int    `json:"w"`
				H    int    `json:"h"`
			} `json:"icons"`
		} `json:"atlases"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	seenScales := map[int]bool{}
	for _, atlas := range manifest.Atlases {
		if atlas.Scale != 1 && atlas.Scale != 2 {
			continue
		}
		if seenScales[atlas.Scale] {
			t.Fatalf("duplicate icon atlas scale %d", atlas.Scale)
		}
		seenScales[atlas.Scale] = true
		file, err := res.FS().Open("icons/" + atlas.File)
		if err != nil {
			t.Fatal(err)
		}
		image, err := png.Decode(file)
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range ids {
			for _, size := range []int{16, 24} {
				found := 0
				for _, icon := range atlas.Icons {
					if icon.ID != id || icon.Size != size {
						continue
					}
					found++
					if icon.W != size*atlas.Scale || icon.H != size*atlas.Scale || icon.X < 0 || icon.Y < 0 || icon.X+icon.W > image.Bounds().Dx() || icon.Y+icon.H > image.Bounds().Dy() {
						t.Fatalf("%s %d@%dx has an invalid region: %+v", id, size, atlas.Scale, icon)
					}
					ink := false
					for y := icon.Y; y < icon.Y+icon.H && !ink; y++ {
						for x := icon.X; x < icon.X+icon.W; x++ {
							_, _, _, alpha := image.At(x, y).RGBA()
							ink = ink || alpha != 0
						}
					}
					if !ink {
						t.Errorf("%s %d@%dx is blank", id, size, atlas.Scale)
					}
					if id == "file.art" && size == 16 && atlas.Scale == 1 {
						_, _, _, pencil := image.At(icon.X+6, icon.Y+10).RGBA()
						_, _, _, paper := image.At(icon.X+4, icon.Y+8).RGBA()
						if pencil != 0 || paper == 0 {
							t.Error("art document icon lost its pencil cutout")
						}
					}
				}
				if found != 1 {
					t.Errorf("%s %d@%dx: found %d atlas regions", id, size, atlas.Scale, found)
				}
			}
		}
	}
	if !seenScales[1] || !seenScales[2] {
		t.Fatalf("missing 1x or 2x atlas: %v", seenScales)
	}
}
