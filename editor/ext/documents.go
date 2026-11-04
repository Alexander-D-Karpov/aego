package ext

import (
	"fmt"
	"slices"

	"aego/editor/core/document"
	"aego/editor/core/workspace"
)

// DocumentHandler registers an asset opening route owned by this extension.
// Disabling the extension removes its route; assets still have the read-only
// fallback. No Asset Browser knowledge of the module's kind is necessary.
func (r *Registry) DocumentHandler(h document.Handler) error {
	ed := r.deps.Editor
	if ed == nil || ed.Docs == nil {
		return fmt.Errorf("document handler needs a document manager")
	}
	owner := r.ext()
	open := h.Open
	if open != nil {
		h.Open = func(d *document.AssetDocument) (err error) {
			defer func() {
				if rec := recover(); rec != nil {
					r.Disable(owner, fmt.Sprint(rec))
					err = fmt.Errorf("document handler %s panicked: %v", h.ID, rec)
				}
			}()
			return open(d)
		}
	}
	if err := ed.Docs.Handlers().Register(h); err != nil {
		return err
	}
	r.own(owner).documents = append(r.own(owner).documents, h.ID)
	return nil
}

// ShowPanel activates a registered presentation in the existing workspace.
// The handler's document identity stays in DocumentManager.
func (r *Registry) ShowPanel(id string) {
	if r.deps.Workspace != nil {
		r.deps.Workspace.Show(id)
	}
}

// DocumentPanelFactory captures the extension owner at registration time. It
// creates an ordinary workspace tab on first open and activates the same tab
// thereafter. The factory must derive its panel ID from the asset identity;
// the mutable model and presentation state remain in AssetDocument.
func (r *Registry) DocumentPanelFactory(factory func(*document.AssetDocument) workspace.Panel) func(*document.AssetDocument) error {
	owner := r.ext()
	return func(d *document.AssetDocument) error {
		if reason, off := r.disabled[owner]; off {
			return fmt.Errorf("document extension is disabled: %s", reason)
		}
		if factory == nil || r.deps.Workspace == nil {
			return fmt.Errorf("document panel requires a workspace and factory")
		}
		p := factory(d)
		if p == nil {
			return fmt.Errorf("document panel factory returned no panel")
		}
		id := p.ID()
		if !r.deps.Workspace.Has(id) {
			if err := r.deps.Workspace.Register(workspace.Desc{Panel: r.wrapPanel(p, owner), Slot: workspace.SlotCenter, Hidden: true, Icon: d.Icon, Order: extOrder}); err != nil {
				return err
			}
			r.own(owner).panels = append(r.own(owner).panels, id)
		}
		r.deps.Workspace.Focus(id)
		return nil
	}
}

func (r *Registry) ClosePanel(id string) {
	if r.deps.Workspace != nil {
		r.deps.Workspace.Unregister(id)
	}
	for _, own := range r.owned {
		if i := slices.Index(own.panels, id); i >= 0 {
			own.panels = slices.Delete(own.panels, i, i+1)
		}
	}
}

func (r *Registry) wrapPanel(p workspace.Panel, owner string) workspace.Panel {
	s := &safePanel{p: p, ext: owner, reg: r}
	if _, ok := p.(workspace.Closer); ok {
		return &safeClosingPanel{safePanel: s}
	}
	return s
}
