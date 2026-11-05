package agent

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image/png"
	"io"
	"os"

	"aego/core/mathx"
	"aego/editor/core/agentui"
	"aego/platform/input"
	"aego/render/texture"
	"aego/ui/core"
	"aego/ui/widgets"
)

const attachmentDir = agentui.Dir + "/images"

type displayImage struct {
	key    string
	region texture.Region
}

type imageViewer struct {
	data   []byte
	region texture.Region
	err    string
	opened uint64
}

var imageViewerID = core.IDOf("agent.image-viewer")

func (p *Panel) storeImages(images [][]byte) []string {
	if len(images) == 0 || p.d.Root == "" {
		return nil
	}
	root, err := os.OpenRoot(p.d.Root)
	if err != nil {
		return nil
	}
	defer root.Close()
	if err = root.MkdirAll(attachmentDir, 0700); err != nil {
		return nil
	}
	var keys []string
	for _, data := range images {
		if len(data) > 5<<20 {
			continue
		}
		key := fmt.Sprintf("%x", sha256.Sum256(data))
		path := attachmentDir + "/" + key + ".png"
		f, err := root.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err == nil {
			_, err = f.Write(data)
			closeErr := f.Close()
			if err != nil || closeErr != nil {
				_ = root.Remove(path)
				continue
			}
		} else if !errors.Is(err, os.ErrExist) {
			continue
		}
		keys = append(keys, key)
	}
	return keys
}
func (p *Panel) dropImages() {
	if p.d.Device != nil {
		for _, i := range p.imageCache {
			p.d.Device.DestroyTexture(i.region.Texture)
		}
	}
	p.imageCache = nil
}

func (p *Panel) readImage(key string) (texture.Region, error) {
	for _, v := range p.imageCache {
		if v.key == key {
			return v.region, nil
		}
	}
	if p.d.Device == nil {
		return texture.Region{}, errors.New("image display needs a GPU")
	}
	buf, err := p.readImageBytes(key)
	if err != nil {
		return texture.Region{}, err
	}
	reg, err := p.uploadThumbnail(buf)
	if err != nil {
		return texture.Region{}, err
	}
	if len(p.imageCache) >= 32 {
		p.d.Device.DestroyTexture(p.imageCache[0].region.Texture)
		p.imageCache = p.imageCache[1:]
	}
	p.imageCache = append(p.imageCache, displayImage{key, reg})
	return reg, nil
}

func (p *Panel) readImageBytes(key string) ([]byte, error) {
	if len(key) != 64 {
		return nil, errors.New("invalid image key")
	}
	if _, err := hex.DecodeString(key); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(p.d.Root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.Open(attachmentDir + "/" + key + ".png")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf, err := io.ReadAll(io.LimitReader(f, (5<<20)+1))
	if err != nil {
		return nil, err
	}
	if fmt.Sprintf("%x", sha256.Sum256(buf)) != key {
		return nil, errors.New("image attachment changed")
	}
	c, err := png.DecodeConfig(bytes.NewReader(buf))
	if err != nil || c.Width < 1 || c.Height < 1 || c.Height > (16<<20)/c.Width {
		return nil, errors.New("invalid image attachment")
	}
	return buf, nil
}
func (p *Panel) drawImage(ui *core.Context, key string, r mathx.Rect) {
	reg, err := p.readImage(key)
	if err != nil {
		ui.TextIn(r, err.Error(), ui.Theme.Warn, core.AlignCenter)
		return
	}
	size := reg.Size()
	scale := min(r.W/size.X, r.H/size.Y)
	w, h := size.X*scale, size.Y*scale
	ui.List().Texture(reg, mathx.Rect{X: r.X + (r.W-w)/2, Y: r.Y + (r.H-h)/2, W: w, H: h}, mathx.White)
	id := ui.ID("agent-image/" + key)
	if ui.Clicked(id, r) {
		if data, err := p.readImageBytes(key); err == nil {
			p.openImage(ui, data)
		} else {
			p.notice(err.Error())
		}
	}
	if ui.Hot() == id {
		ui.SetCursorShape(core.CursorPointer)
	}
}

func (p *Panel) openImage(ui *core.Context, data []byte) {
	if ui.ModalActive() || p.d.Device == nil {
		return
	}
	c, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || len(data) > 5<<20 || c.Width < 1 || c.Height < 1 || c.Height > (16<<20)/c.Width {
		p.notice("invalid image attachment")
		return
	}
	p.closeImageViewer(nil)
	p.imageViewer.data = bytes.Clone(data)
	p.imageViewer.opened = ui.Frame()
	ui.CancelFocus()
	ui.OpenModal(imageViewerID)
}

func (p *Panel) closeImageViewer(ui *core.Context) {
	if p.d.Device != nil && !p.imageViewer.region.Texture.IsZero() {
		p.d.Device.DestroyTexture(p.imageViewer.region.Texture)
	}
	p.imageViewer = imageViewer{}
	if ui != nil && ui.ModalOpen(imageViewerID) {
		ui.CloseModal()
		p.focus = true
	}
}

// DrawImageViewer is called by the shell outside panel clips, even when the panel is hidden.
func (p *Panel) DrawImageViewer(ui *core.Context) {
	if p.imageViewer.data == nil {
		return
	}
	if !ui.ModalOpen(imageViewerID) {
		p.closeImageViewer(nil)
		return
	}
	sz, th := ui.Size(), &ui.Theme
	w, h := max(1, sz.X-40), max(1, sz.Y-40)
	r := mathx.Rect{X: (sz.X - w) / 2, Y: (sz.Y - h) / 2, W: w, H: h}
	if ui.KeyPressed(input.KeyEscape) || (ui.Frame() != p.imageViewer.opened && ui.MousePressed(input.MouseLeft) && !r.Contains(ui.Mouse())) {
		p.closeImageViewer(ui)
		return
	}
	if !ui.BeginModal(imageViewerID, r) {
		return
	}
	ui.PushRawID(imageViewerID)
	bar := ui.Cursor().Next(0)
	closeRect := mathx.Rect{X: bar.X + bar.W - bar.H, Y: bar.Y, W: bar.H, H: bar.H}
	close := widgets.ButtonRect(ui, ui.ID("close"), closeRect, "×")
	area := mathx.Rect{X: r.X + th.Padding, Y: bar.Y + bar.H + th.Spacing, W: max(0, r.W-2*th.Padding), H: max(0, r.Y+r.H-th.Padding-(bar.Y+bar.H+th.Spacing))}
	if p.imageViewer.region.Texture.IsZero() && p.imageViewer.err == "" {
		data, err := texture.DecodePNG(bytes.NewReader(p.imageViewer.data), true)
		if err == nil {
			var tex texture.Region
			var handleErr error
			handle, handleErr := texture.NewUploader(p.d.Device).Upload(data, texture.UploadOptions{Label: "agent.image-viewer"})
			if handleErr == nil {
				tex = texture.Whole(handle, data.Width, data.Height)
				p.imageViewer.region = tex
			}
			err = handleErr
		}
		if err != nil {
			p.imageViewer.err = err.Error()
		}
	}
	if p.imageViewer.err != "" {
		ui.TextIn(area, p.imageViewer.err, th.Warn, core.AlignCenter)
	} else if reg := p.imageViewer.region; !reg.Texture.IsZero() {
		size := reg.Size()
		k := min(area.W/size.X, area.H/size.Y)
		w, h := size.X*k, size.Y*k
		ui.List().Texture(reg, mathx.Rect{X: area.X + (area.W-w)/2, Y: area.Y + (area.H-h)/2, W: w, H: h}, mathx.White)
	}
	ui.PopID()
	ui.EndModal()
	if close {
		p.closeImageViewer(ui)
	}
}

func (p *Panel) uploadThumbnail(buf []byte) (texture.Region, error) {
	data, err := texture.DecodePNG(bytes.NewReader(buf), true)
	if err != nil {
		return texture.Region{}, err
	}
	// Keep only a display-sized copy on the GPU. The transcript retains the original PNG.
	if n := max(data.Width, data.Height); n > 512 {
		w, h := max(1, data.Width*512/n), max(1, data.Height*512/n)
		small := texture.New(w, h, data.Format)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				i := 4 * ((y*data.Height/h)*data.Width + x*data.Width/w)
				copy(small.Pixels[4*(y*w+x):4*(y*w+x)+4], data.Pixels[i:i+4])
			}
		}
		data = small
	}
	tex, err := texture.NewUploader(p.d.Device).Upload(data, texture.UploadOptions{Label: "agent.tool-image"})
	if err != nil {
		return texture.Region{}, err
	}
	reg := texture.Whole(tex, data.Width, data.Height)
	return reg, nil
}
