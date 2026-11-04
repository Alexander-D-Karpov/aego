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
	"aego/render/texture"
	"aego/ui/core"
)

const attachmentDir = agentui.Dir + "/images"

type displayImage struct {
	key    string
	region texture.Region
}

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
	if len(key) != 64 {
		return texture.Region{}, errors.New("invalid image key")
	}
	if _, err := hex.DecodeString(key); err != nil {
		return texture.Region{}, err
	}
	root, err := os.OpenRoot(p.d.Root)
	if err != nil {
		return texture.Region{}, err
	}
	defer root.Close()
	f, err := root.Open(attachmentDir + "/" + key + ".png")
	if err != nil {
		return texture.Region{}, err
	}
	defer f.Close()
	buf, err := io.ReadAll(io.LimitReader(f, (5<<20)+1))
	if err != nil {
		return texture.Region{}, err
	}
	if fmt.Sprintf("%x", sha256.Sum256(buf)) != key {
		return texture.Region{}, errors.New("image attachment changed")
	}
	c, err := png.DecodeConfig(bytes.NewReader(buf))
	if err != nil || c.Width < 1 || c.Height < 1 || c.Height > (16<<20)/c.Width {
		return texture.Region{}, errors.New("invalid image attachment")
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
