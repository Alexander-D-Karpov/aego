package agentbridge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"image/png"
	"strings"
	"time"

	"aego/app/agent"
	"aego/app/agent/mcp"
	ed "aego/editor/core/context"
	"aego/engine/i18n"
)

type variantSet struct {
	generation uint64
	bytes      map[string][]byte
}

func (b *Bridge) generatorTool() mcp.Tool {
	return mcp.Tool{Name: "asset_generate_variants", Description: "Use the connected image generator with explicit approval, a maximum total cost in USD cents, and up to three selected project PNG references. Returns PNG variants for inspection. Import a choice separately using asset_import_variant. Use asset_paint without a generator.", Schema: visualSchema([]string{"prompt", "maxCents"}, map[string]any{"prompt": map[string]any{"type": "string"}, "maxCents": map[string]any{"type": "integer", "minimum": 0, "maximum": 10000}, "count": map[string]any{"type": "integer", "minimum": 1, "maximum": 3}, "references": map[string]any{"type": "array", "maxItems": 3, "items": map[string]any{"type": "string"}}}), Output: b.generateVariants}
}
func (b *Bridge) generateVariants(args map[string]any) (*agent.Output, error) {
	var g ed.ImageGenerator
	if e := b.main(func() { g = b.ctx.ImageGenerator }); e != nil {
		return nil, e
	}
	if g == nil {
		return nil, errors.New("no external image generator connected; asset_paint and asset_pixels are available")
	}
	a := agent.Args(args)
	gen := b.panelGeneration.Load()
	prompt := strings.TrimSpace(a.Str("prompt"))
	limit, err := a.Int("maxCents", -1)
	if err != nil || limit < 0 || limit > 10000 || len(prompt) == 0 || len(prompt) > 6000 {
		return nil, errors.New("supply a prompt and a maximum total cost in USD cents")
	}
	n, err := a.Int("count", 1)
	if err != nil || n < 1 || n > 3 {
		return nil, errors.New("generate 1 to 3 variants")
	}
	req := ed.ImageGeneration{Prompt: prompt, Count: n, MaxCents: limit}
	var names []string
	if v, ok := args["references"]; ok {
		rs, ok := v.([]any)
		if !ok || len(rs) > 3 {
			return nil, errors.New("at most three project image references")
		}
		for _, raw := range rs {
			s, ok := raw.(string)
			if !ok {
				return nil, errors.New("invalid reference")
			}
			res, e := b.viewAsset(map[string]any{"asset": s, "view": "source"})
			if e != nil {
				return nil, e
			}
			names = append(names, s)
			req.References = append(req.References, res.Images[0].Data)
		}
	}
	if err = b.permit(i18n.Tr("editor.agent.generate_images", "Generate {n} image(s) with {provider}? Maximum total: {cost} USD cents. References: {refs}\n{prompt}", i18n.Int("n", int64(n)), i18n.Str("provider", g.Name()), i18n.Int("cost", int64(limit)), i18n.Str("refs", strings.Join(names, ", ")), i18n.Str("prompt", prompt)), gen); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	go func() {
		t := time.NewTicker(100 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if b.cancelled(gen) {
					cancel()
					return
				}
			}
		}
	}()
	data, err := g.Generate(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(data) < 1 || len(data) > n {
		return nil, errors.New("invalid generator output count")
	}
	variants := variantSet{gen, map[string][]byte{}}
	var keys []string
	var imgs []agent.Image
	for _, raw := range data {
		cfg, err := png.DecodeConfig(bytes.NewReader(raw))
		if err != nil || len(raw) > 5<<20 || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 4096 || cfg.Height > 4096 || cfg.Height > (4<<20)/cfg.Width {
			return nil, errors.New("generated PNG exceeds input limits or is invalid")
		}
		if _, err = png.Decode(bytes.NewReader(raw)); err != nil {
			return nil, err
		}
		key := fmt.Sprintf("%x", sha256.Sum256(raw))
		owned := bytes.Clone(raw)
		variants.bytes[key] = owned
		keys = append(keys, key)
		imgs = append(imgs, agent.Image{Mime: "image/png", Data: owned})
	}
	if err = b.main(func() {
		if !b.cancelled(gen) {
			b.variants = variants
		}
	}); err != nil {
		return nil, err
	}
	if b.cancelled(gen) {
		return nil, errCancelled
	}
	return &agent.Output{OK: true, Value: map[string]any{"variantIDs": keys, "provider": g.Name()}, Images: imgs}, nil
}
func (b *Bridge) GeneratedImage(id string) ([]byte, bool) {
	if b.variants.generation != b.panelGeneration.Load() {
		return nil, false
	}
	v, ok := b.variants.bytes[id]
	return v, ok
}
