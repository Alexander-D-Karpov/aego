package hotreload

import (
	"os"
	"path/filepath"
	"time"

	"aego/app/run"
	"aego/asset"
	"aego/asset/loader"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/demos"
	"aego/demos/internal/app"
	"aego/engine"
	"aego/platform/input"
	"aego/project"
	"aego/render"
	renderhost "aego/render/host"
	"aego/render/sprite"
	"aego/render/texture"
	"aego/ui/core"
	"aego/ui/widgets"
)

func init() {
	demos.Register("asset-hot-reload", func() demos.Demo { return New() })
}

type Demo struct {
	app.Base
	mod      *renderhost.Module
	project  *project.Project
	pipeline *pipeline.Pipeline
	loader   *loader.Loader
	stop     func()
	spriteID ids.AssetID
	region   texture.Region
	reloads  int
	lastErr  string
	assetDir string
	variant  int
}

func New() *Demo {
	return &Demo{}
}

func (d *Demo) Name() string {
	return "asset-hot-reload"
}

func (d *Demo) Module() *renderhost.Module {
	if d.mod == nil {
		d.mod = renderhost.New(render.Options{MaxSprites: 1024})
	}
	return d.mod
}

func (d *Demo) Init(e *engine.Engine) error {
	d.Attach(e, d.Module())
	root, err := os.MkdirTemp("", "aego-hotreload-*")
	if err != nil {
		return err
	}
	p, err := project.Create(root, "HotReload", false)
	if err != nil {
		return err
	}
	d.project = p
	d.assetDir = p.AssetsDir()
	if err := d.writeSprite(0); err != nil {
		return err
	}

	imps, err := run.DefaultImporters()
	if err != nil {
		return err
	}
	d.pipeline, err = pipeline.Open(p, imps, e.Log())
	if err != nil {
		return err
	}
	if _, _, err := d.pipeline.Scan(); err != nil {
		return err
	}
	batch, err := d.pipeline.ImportAll()
	if err != nil {
		return err
	}
	if len(batch.Imported) > 0 {
		d.spriteID = batch.Imported[0].ID
	}

	d.loader = loader.New(e.GPU(), d.Renderer.Textures(), e.Log())
	if err := d.loader.Mount(d.pipeline.Index(), d.pipeline.Artifacts()); err != nil {
		return err
	}
	d.loader.Subscribe(func(r asset.Reloaded) {
		d.reloads++
	})
	d.stop, err = d.pipeline.Watch(150*time.Millisecond, func(b *pipeline.Batch) {
		e.Jobs().OnMain(func() { d.apply(b) })
	})
	return err
}

func (d *Demo) writeSprite(variant int) error {
	colors := [][3]uint8{
		{0xE0, 0x7B, 0x39},
		{0x52, 0xA7, 0xC9},
		{0x8A, 0xC9, 0x52},
	}
	c := colors[variant%len(colors)]
	data := texture.Checker(32, 32,
		8,
		mathx.ColorRGB8(c[0], c[1], c[2]),
		mathx.ColorRGB8(0x20, 0x24, 0x2c),
	)
	png, err := encodePNG(data)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(d.assetDir, "player.png"), png, 0o644)
}

func (d *Demo) writeBroken() error {
	return os.WriteFile(filepath.Join(d.assetDir, "player.png"), []byte("this is not a png"), 0o644)
}

func (d *Demo) apply(b *pipeline.Batch) {
	if len(b.Failed) > 0 {
		for _, err := range b.Failed {
			d.lastErr = err.Error()
		}
		return
	}
	if err := d.loader.Swap(b.Imported, d.pipeline.Artifacts()); err != nil {
		d.lastErr = err.Error()
		return
	}
	d.lastErr = ""
	d.region = texture.Region{}
}

func (d *Demo) Update(float64) {
	d.ToggleOverlay()
	in := d.Engine.Input()
	if in.KeyPressed(input.KeyEscape) {
		d.Engine.Quit()
	}
	if in.KeyPressed(input.KeyR) {
		d.variant++
		if err := d.writeSprite(d.variant); err != nil {
			d.lastErr = err.Error()
		}
	}
	if in.KeyPressed(input.KeyX) {
		if err := d.writeBroken(); err != nil {
			d.lastErr = err.Error()
		}
	}
}

func (d *Demo) Render(float64) {
	if d.region.IsZero() && !d.spriteID.IsZero() {
		if reg, ok := d.loader.Texture(d.spriteID); ok {
			d.region = reg
		}
	}
	v := d.WorldView()
	v.Camera.Zoom = 96
	f := d.Renderer.View(v)
	region := d.region
	if region.IsZero() {
		region = d.Renderer.White()
	}
	f.Sprite(sprite.Draw{
		Region: region,
		Size:   mathx.Vec2{X: 3, Y: 3},
		Origin: mathx.Vec2{X: 0.5, Y: 0.5},
		Tint:   mathx.Color{R: 1, G: 1, B: 1, A: 1},
	})
	f.End()

	d.BeginUI()
	d.DrawOverlay(func(c *core.Context) {
		widgets.LabelDim(c, "R writes a new sprite, X writes a broken file")
		widgets.LabelDim(c, d.assetDir)
		widgets.Labelf(c, "reloads %d", d.reloads)
		if d.lastErr != "" {
			widgets.Label(c, "last error (old sprite kept):")
			widgets.LabelWrapped(c, d.lastErr, true)
		}
	})
	d.FlushUI()
}

func (d *Demo) Shutdown() {
	if d.stop != nil {
		d.stop()
	}
	if d.loader != nil {
		d.loader.Close()
	}
	if d.pipeline != nil {
		d.pipeline.Close()
	}
	if d.project != nil {
		os.RemoveAll(d.project.Root)
	}
	_ = log.Nop()
}
