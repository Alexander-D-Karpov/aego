package assembly

import "aego/engine/module"

type hooked struct {
	inner module.Module
	hook  any
}

func (h hooked) Info() module.Info {
	return h.inner.Info()
}

func (h hooked) Register(r *module.Registry) error {
	if err := h.inner.Register(r); err != nil {
		return err
	}
	r.Editor(h.hook)
	return nil
}

func (h hooked) Init(ctx *module.InitContext) error {
	if in, ok := h.inner.(module.Initer); ok {
		return in.Init(ctx)
	}
	return nil
}

func (h hooked) AttachWorld(ctx *module.AttachContext) error {
	if at, ok := h.inner.(module.WorldAttacher); ok {
		return at.AttachWorld(ctx)
	}
	return nil
}

func (h hooked) DetachWorld(ctx *module.AttachContext) {
	if at, ok := h.inner.(module.WorldAttacher); ok {
		at.DetachWorld(ctx)
	}
}

func (h hooked) Shutdown() {
	if sd, ok := h.inner.(module.Shutdowner); ok {
		sd.Shutdown()
	}
}

func (h hooked) Reconfigure(ctx *module.AttachContext) error {
	if rc, ok := h.inner.(module.Reconfigurer); ok {
		return rc.Reconfigure(ctx)
	}
	return nil
}
