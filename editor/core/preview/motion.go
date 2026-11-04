package preview

import (
	"aego/asset"
	"aego/core/ids"
	"aego/editor/core/tasks"
	"fmt"
	"image"
)

type Motion struct {
	IDs    []ids.AssetID
	Times  []float32
	Length float32
}
type Card struct {
	Entry  asset.Entry
	Source []byte
	Image  *image.NRGBA
	s      *Service
}

func (s *Service) input(e asset.Entry, src []byte) *Card { return &Card{e, src, canvas(), s} }
func (c *Card) Artifact() ([]byte, error) {
	a, ok := c.Entry.Artifact(c.Entry.Kind)
	if !ok || c.s.d.Pipeline == nil {
		return nil, fmt.Errorf("missing %s artifact", c.Entry.Kind)
	}
	return c.s.d.Pipeline.Artifacts().ReadFile(a.Path)
}
func (c *Card) Blit(id ids.AssetID, area image.Rectangle) (string, error) {
	// Cards run on workers. Source files are editor-thread-only and are never
	// image artifacts; do not read the mutable source map via Service.entry.
	var e asset.Entry
	var ok bool
	if b, found := asset.LookupBuiltin(id); found {
		e, ok = asset.Entry{ID: id, Kind: b.Kind, Name: b.Name, Source: b.Name}, true
	} else if c.s.d.Pipeline != nil && !id.IsZero() {
		e, ok = c.s.d.Pipeline.Entry(id)
	}
	if !ok {
		return "", fmt.Errorf("missing asset %s", id.Short())
	}
	if !Image(e.Kind) {
		return "", fmt.Errorf("%s has no image or waveform", e.Name)
	}
	p, err := c.s.buildImage(e)
	if err == nil {
		fitPixels(c.Image, area, p)
	}
	return e.Source, err
}
func (c *Card) Label(x, y int, text string, highlight bool) {
	col := ink
	if highlight {
		col = accent
	}
	c.s.label(c.Image, x, y, text, col)
}
func (s *Service) HoverFrame(id ids.AssetID, seconds float64) (ids.AssetID, bool) {
	it := s.items[id]
	if it == nil || it.state != ready {
		return ids.AssetID{}, false
	}
	if !it.hoverRequested {
		e, ok := s.entry(id)
		if !ok {
			return ids.AssetID{}, false
		}
		it.hoverRequested = true
		switch e.Kind {
		case asset.ArtTexture:
			if s.d.Pipeline != nil {
				if m, ok := s.d.Pipeline.MetaOf(id); ok {
					for i, f := range m.Subs {
						it.motion.IDs = append(it.motion.IDs, f.ID)
						it.motion.Times = append(it.motion.Times, float32(i)/8)
					}
					it.motion.Length = float32(len(m.Subs)) / 8
				}
			}
		default:
			provider := s.handlers[e.Kind].Motion
			if provider == nil {
				return ids.AssetID{}, false
			}
			var src []byte
			if s.d.Docs != nil {
				if d := s.d.Docs.Script(e.Source); d != nil && d.Dirty() {
					src = append([]byte(nil), d.Code.Bytes()...)
				}
			}
			var m Motion
			s.d.Tasks.Work(tasks.Spec{Kind: "preview", Priority: s.frame * 2}, func(*tasks.Task) error {
				var err error
				m, err = provider(s.input(e, src))
				return err
			}).Then(func(t *tasks.Task) {
				if s.items[id] != it {
					return
				}
				it.motionLoaded = true
				if t.State() == tasks.Done {
					it.motion = m
				}
			})
		}
	}
	m := it.motion
	if len(m.IDs) < 2 || m.Length <= 0 {
		return ids.AssetID{}, false
	}
	phase := float32(seconds - float64(int64(seconds/float64(m.Length)))*float64(m.Length))
	i := 0
	for i+1 < len(m.IDs) && m.Times[i+1] <= phase {
		i++
	}
	return m.IDs[i], true
}

func (s *Service) FrameIDs(id ids.AssetID) ([]ids.AssetID, error) {
	it := s.items[id]
	if it == nil || it.state == pending {
		return nil, ErrPending
	}
	e, ok := s.entry(id)
	if !ok {
		return nil, fmt.Errorf("unknown asset")
	}
	if e.Kind != asset.ArtTexture && s.handlers[e.Kind].Motion == nil {
		return nil, fmt.Errorf("asset has no registered frame sequence")
	}
	s.HoverFrame(id, 0)
	if len(it.motion.IDs) == 0 {
		if e.Kind == asset.ArtTexture {
			return nil, fmt.Errorf("image has no authored frames")
		}
		if !it.motionLoaded {
			return nil, ErrPending
		}
		return nil, fmt.Errorf("animation has no sprite frames")
	}
	return append([]ids.AssetID(nil), it.motion.IDs...), nil
}
