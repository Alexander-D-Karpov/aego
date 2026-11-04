package ext

import (
	"aego/editor/core/context"
	"errors"
)

func (r *Registry) ImageGenerator(g context.ImageGenerator) error {
	ed := r.Editor()
	if ed == nil || g == nil || g.Name() == "" {
		return errors.New("invalid image generator")
	}
	if ed.ImageGenerator != nil {
		return errors.New("an image generator is already registered")
	}
	ed.ImageGenerator = g
	r.own(r.ext()).imageGenerator = true
	return nil
}
