package document

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"aego/asset"
	"aego/engine/i18n"
)

// Capabilities describe supported authoring operations, not file extensions.
// In particular, viewing a PNG does not make its source pixels editable.
type Capabilities uint8

const (
	View Capabilities = 1 << iota
	EditSource
	EditMetadata
	Preview
	RevealSource
)

func (c Capabilities) Has(want Capabilities) bool { return c&want == want }

func (c Capabilities) Names() []string {
	var out []string
	for _, p := range []struct {
		bit  Capabilities
		name string
	}{{View, "View"}, {EditSource, "EditSource"}, {EditMetadata, "EditMetadata"}, {Preview, "Preview"}, {RevealSource, "RevealSource"}} {
		if c.Has(p.bit) {
			out = append(out, p.name)
		}
	}
	return out
}

// Handler registers a presentation; TextSource promises UTF-8 source, and Roles specialize same-kind handlers by semantic role.
// The manager neither guesses text from bytes nor opens unregistered binaries in the code editor.
type Handler struct {
	ID           string
	Name         i18n.Text
	Icon         string
	Kinds        []asset.ArtifactKind
	Extensions   []string
	Roles        []string
	// SourceExtensionsOnly specializes a source editor without claiming every
	// prepared resource of the same kind (for example every PNG texture).
	SourceExtensionsOnly bool
	Capabilities Capabilities
	TextSource   bool
	Open         func(*AssetDocument) error
}

type HandlerRegistry struct{ entries []Handler }

func (r *HandlerRegistry) Register(h Handler) error {
	if h.ID == "" || !h.Capabilities.Has(View) || (len(h.Kinds) == 0 && len(h.Extensions) == 0) {
		return fmt.Errorf("document handler requires an ID, a kind or extension, and View capability")
	}
	if h.Name.En == "" {
		h.Name = i18n.Text{En: h.ID}
	}
	if h.Icon == "" {
		h.Icon = "file.unknown"
	}
	for _, old := range r.entries {
		if old.ID == h.ID {
			return fmt.Errorf("document handler %q is already registered", h.ID)
		}
		if slices.Equal(old.Roles, h.Roles) {
			for _, k := range h.Kinds {
				if slices.Contains(old.Kinds, k) {
					if old.SourceExtensionsOnly != h.SourceExtensionsOnly {
						continue
					}
					if old.SourceExtensionsOnly && !sourceExtensionsOverlap(old.Extensions, h.Extensions) {
						continue
					}
					return fmt.Errorf("document kind %q already has handler %q", k, old.ID)
				}
			}
		}
	}
	h.Kinds = slices.Clone(h.Kinds)
	h.Roles = slices.Clone(h.Roles)
	h.Extensions = slices.Clone(h.Extensions)
	for i, e := range h.Extensions {
		if !strings.HasPrefix(e, ".") || strings.ContainsAny(e, "/\\") {
			return fmt.Errorf("invalid document extension %q", e)
		}
		h.Extensions[i] = strings.ToLower(e)
	}
	r.entries = append(r.entries, h)
	return nil
}

func (r *HandlerRegistry) Unregister(id string) {
	for i := range r.entries {
		if r.entries[i].ID == id {
			r.entries = append(r.entries[:i], r.entries[i+1:]...)
			return
		}
	}
}

func (r *HandlerRegistry) All() []Handler { return slices.Clone(r.entries) }

// Resolve prefers semantic role, then kind, then extension if no kind exists; registration order gives no priority, and unhandled formats get a read-only view.
func (r *HandlerRegistry) Resolve(kind asset.ArtifactKind, name, role string) Handler {
	ext := strings.ToLower(path.Ext(name))
	best, score := Handler{}, 0
	for _, h := range r.entries {
		s := 0
		if h.SourceExtensionsOnly {
			if !slices.Contains(h.Extensions, ext) || (kind != "" && !slices.Contains(h.Kinds, kind)) {
				continue
			}
			s = 8
		} else if slices.Contains(h.Kinds, kind) && kind != "" {
			s = 2
		} else if kind == "" && slices.Contains(h.Extensions, ext) {
			s = 1
		}
		if s == 0 {
			continue
		}
		if len(h.Roles) > 0 {
			if !slices.Contains(h.Roles, role) {
				continue
			}
			s += 4
		}
		if s > score || (s == score && h.ID < best.ID) {
			best, score = h, s
		}
	}
	if score != 0 {
		return best
	}
	return Handler{ID: "asset.readonly", Name: i18n.T("editor.document.asset_information", "Asset Information"), Icon: "file.unknown", Capabilities: View | RevealSource}
}
