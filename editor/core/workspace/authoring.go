package workspace

const (
	Animation = "animation"
	VFX       = "vfx"
	Default   = "default"
)

// ResetFactoryLayouts resets only factory layouts, never named user layouts or
// documents. Invoke explicitly after confirmation; opening an asset must not call it.
func (w *Workspace) ResetFactoryLayouts() {
	current := w.Current()
	if current.User {
		w.stash()
	}
	for _, p := range w.presets {
		if !p.User {
			delete(w.layouts, p.ID)
		}
	}
	if !current.User {
		w.apply()
	}
	w.pending = true
}
