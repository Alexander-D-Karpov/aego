package ext

import (
	"aego/asset"
	"aego/editor/core/preview"
	"fmt"
)

func (r *Registry) Thumbnail(kind asset.ArtifactKind, h preview.Handler) error {
	ed := r.Editor()
	if ed == nil {
		return nil
	}
	s, ok := ed.Thumbs.(*preview.Service)
	if !ok {
		return nil
	}
	owner := r.ext()
	render := h.Render
	if render != nil {
		h.Render = func(e asset.Entry, size int) (im preview.Surface, err error) {
			defer func() {
				if rec := recover(); rec != nil {
					err = fmt.Errorf("thumbnail %s: %v", kind, rec)
					r.Disable(owner, err.Error())
				}
			}()
			return render(e, size)
		}
	}
	if build := h.Build; build != nil {
		h.Build = func(c *preview.Card) (ok bool, err error) {
			defer func() {
				if rec := recover(); rec != nil {
					ok, err = true, fmt.Errorf("%s preview: %v", owner, rec)
				}
			}()
			return build(c)
		}
	}
	if motion := h.Motion; motion != nil {
		h.Motion = func(c *preview.Card) (m preview.Motion, err error) {
			defer func() {
				if rec := recover(); rec != nil {
					err = fmt.Errorf("%s hover preview: %v", owner, rec)
				}
			}()
			return motion(c)
		}
	}
	if err := s.Register(kind, h); err != nil {
		return err
	}
	r.own(owner).thumbs = append(r.own(owner).thumbs, thumbnailRegistration{kind, h.Component})
	return nil
}
