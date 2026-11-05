package agent

import (
	"bytes"
	"image/png"
	"slices"
	"strconv"
	"strings"

	"aego/core/mathx"
	"aego/engine/i18n"
	"aego/platform/clip"
	"aego/render/texture"
	"aego/ui/core"
	"aego/ui/widgets"
)

type draftImage struct {
	png    []byte
	region texture.Region
}

func imageMarker(i int) string { return "[Image " + strconv.Itoa(i+1) + "]" }

func (p *Panel) removeDraftImage(i int) {
	p.composer.ReplaceFirst(&p.input, imageMarker(i), "")
	if p.d.Device != nil && !p.draftImages[i].region.Texture.IsZero() {
		p.d.Device.DestroyTexture(p.draftImages[i].region.Texture)
	}
	p.draftImages = slices.Delete(p.draftImages, i, i+1)
	for j := i; j < len(p.draftImages); j++ {
		p.composer.ReplaceFirst(&p.input, imageMarker(j+1), imageMarker(j))
	}
}

func (p *Panel) dropMissingDraftImages() {
	for i := 0; i < len(p.draftImages); {
		if !strings.Contains(p.input, imageMarker(i)) {
			p.removeDraftImage(i)
		} else {
			i++
		}
	}
}
func (p *Panel) clearDraftImages() {
	for len(p.draftImages) > 0 {
		p.removeDraftImage(len(p.draftImages) - 1)
	}
}
func (p *Panel) pasteImage(ui *core.Context) bool {
	cb, ok := ui.Clipboard.(clip.Reader)
	if !ok {
		return false
	}
	probe, hasFormats := cb.(clip.Prober)
	var data []byte
	for _, m := range []string{clip.MimePNG, clip.MimeBMP, clip.MimeXBMP} {
		// SDL on X11 can return clipboard text for an unadvertised image
		// type. Leave a text paste to the focused composer in that case.
		if hasFormats && !probe.Has(m) {
			continue
		}
		buf, ok := cb.Data(m)
		if !ok || len(buf) == 0 {
			continue
		}
		if len(p.draftImages) >= 3 {
			p.notice(i18n.Tr("editor.agent.image_count", "A message can include up to three screenshots."))
			return true
		}
		if m == clip.MimePNG {
			data = buf
		} else if len(buf) <= 32<<20 {
			bmp, e := clip.DecodeBMP(buf)
			if e == nil {
				data = clip.EncodePNG(bmp)
			}
		}
		cfg, e := png.DecodeConfig(bytes.NewReader(data))
		if e != nil || len(data) > 5<<20 || cfg.Width < 1 || cfg.Height < 1 || cfg.Height > (16<<20)/cfg.Width {
			p.notice(i18n.Tr("editor.agent.image_size", "Screenshot must fit within 5 MiB and 16 megapixels."))
			return true
		}
		if p.d.Device == nil {
			p.notice(i18n.Tr("editor.agent.image_device", "Image preview unavailable."))
			return true
		}
		reg, e := p.uploadThumbnail(data)
		if e != nil {
			p.notice(e.Error())
			return true
		}
		p.draftImages = append(p.draftImages, draftImage{bytes.Clone(data), reg})
		p.composer.Insert(&p.input, imageMarker(len(p.draftImages)-1))
		return true
	}
	return false
}
func (p *Panel) draftAttachments(ui *core.Context, r mathx.Rect) {
	w := min(float32(100), (r.W-8)/float32(max(1, len(p.draftImages))))
	for i, img := range p.draftImages {
		tile := mathx.Rect{X: r.X + float32(i)*(w+4), Y: r.Y, W: w, H: r.H - 3}
		ui.Box(tile, ui.Theme.Radius.Field, ui.Theme.PanelAlt, ui.Theme.Border)
		s := img.region.Size()
		k := min((tile.W-4)/s.X, (tile.H-4)/s.Y)
		if k > 0 {
			dw, dh := s.X*k, s.Y*k
			ui.List().Texture(img.region, mathx.Rect{X: tile.X + (tile.W-dw)/2, Y: tile.Y + (tile.H-dh)/2, W: dw, H: dh}, mathx.White)
		}
		d := min(ui.Theme.RowHeight, tile.W)
		id := core.IDSeq(ui.ID("remove-image"), i)
		if widgets.ButtonRect(ui, id, mathx.Rect{X: tile.X + tile.W - d, Y: tile.Y, W: d, H: d}, "×") {
			p.removeDraftImage(i)
			break
		}
		widgets.Tooltip(ui, id, i18n.Tr("editor.agent.remove_image", "Remove screenshot")+" "+strconv.Itoa(i+1))
		if ui.Clicked(core.IDSeq(ui.ID("open-draft-image"), i), tile) {
			p.openImage(ui, img.png)
		}
	}
}
