package animation

import (
	"aego/anim"
	"aego/asset"
	"aego/editor/core/preview"
	"aego/editor/ext"
	"aego/format/aetext"
	"fmt"
	"image"
	"image/color"
	"image/draw"
)

func RegisterThumbnails(r *ext.Registry) error {
	if err := r.Thumbnail(asset.ArtAnim, preview.Handler{Version: "clip.v1", Build: clipCard, Motion: motion}); err != nil {
		return err
	}
	return r.Thumbnail(asset.ArtCtrl, preview.Handler{Version: "graph.v1", Build: graphCard})
}
func decodeClip(c *preview.Card) (*anim.Clip, error) {
	src := c.Source
	if src == nil {
		if x, ok := asset.BuiltinAnimText(c.Entry.ID); ok {
			src = []byte(x)
		}
	}
	if src != nil {
		n, err := aetext.Parse(src, c.Entry.Source)
		if err != nil {
			return nil, err
		}
		return anim.Decode(n)
	}
	buf, err := c.Artifact()
	if err != nil {
		return nil, err
	}
	return anim.Unmarshal(buf)
}
func motion(c *preview.Card) (preview.Motion, error) {
	a, err := decodeClip(c)
	if err != nil {
		return preview.Motion{}, err
	}
	return clipFrames(a), nil
}
func clipFrames(a *anim.Clip) preview.Motion {
	m := preview.Motion{Length: a.Length}
	for _, track := range a.Tracks {
		for _, k := range track.Keys {
			if !k.V.A.IsZero() {
				m.IDs = append(m.IDs, k.V.A)
				m.Times = append(m.Times, k.T)
			}
		}
		if len(m.IDs) > 0 {
			break
		}
	}
	return m
}

var graphInk = image.NewUniform(color.NRGBA{R: 99, G: 172, B: 220, A: 255})

func clipCard(c *preview.Card) (bool, error) {
	a, err := decodeClip(c)
	if err != nil {
		return true, err
	}
	frames := clipFrames(a).IDs
	if len(frames) > 0 {
		n := min(len(frames), 4)
		for i := 0; i < n; i++ {
			id := frames[i*(len(frames)-1)/max(1, n-1)]
			x, y := i%2*64, i/2*55
			if _, err := c.Blit(id, image.Rect(x, y, x+64, y+54)); err != nil {
				return true, err
			}
		}
	} else {
		for i, t := range a.Tracks[:min(4, len(a.Tracks))] {
			y := i * 26
			c.Label(4, y, t.Property, false)
			for _, k := range t.Keys {
				x := 4 + int(k.T/max(a.Length, .0001)*118)
				draw.Draw(c.Image, image.Rect(x, y+14, x+3, y+20), graphInk, image.Point{}, draw.Src)
			}
		}
	}
	c.Label(4, 112, fmt.Sprintf("%.2fs / %d tracks", a.Length, len(a.Tracks)), true)
	return true, nil
}
func graphCard(c *preview.Card) (bool, error) {
	var g *anim.Graph
	var err error
	src := c.Source
	if src == nil {
		if x, ok := asset.BuiltinCtrlText(c.Entry.ID); ok {
			src = []byte(x)
		}
	}
	if src != nil {
		var n *aetext.Node
		n, err = aetext.Parse(src, c.Entry.Source)
		if err == nil {
			g, _, err = anim.DecodeGraph(n)
		}
	} else {
		var a []byte
		a, err = c.Artifact()
		if err == nil {
			g, err = anim.UnmarshalGraph(a)
		}
	}
	if err != nil {
		return true, err
	}
	for i, state := range g.States[:min(7, len(g.States))] {
		y := 4 + i*17
		draw.Draw(c.Image, image.Rect(8, y+4, 15, y+10), graphInk, image.Point{}, draw.Src)
		c.Label(19, y, state.Name, i == int(g.Entry))
	}
	for i, tr := range g.Transitions {
		if tr.From < 0 || tr.From >= 7 || tr.To < 0 || tr.To >= 7 {
			continue
		}
		x := 2 + i%5
		y, z := 11+int(tr.From)*17, 11+int(tr.To)*17
		for _, r := range []image.Rectangle{image.Rect(x, min(y, z), x+1, max(y, z)+1), image.Rect(x, y, 9, y+1), image.Rect(x, z, 9, z+1)} {
			draw.Draw(c.Image, r, graphInk, image.Point{}, draw.Src)
		}
	}
	return true, nil
}
