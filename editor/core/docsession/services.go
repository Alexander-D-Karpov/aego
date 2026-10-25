//go:build aego_editor

package docsession

import (
	"bytes"
	"image"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/document"
	"aego/editor/core/selection"
	"aego/editor/core/services"
	"aego/editor/core/templates"
)

func (s *Session) Doc() *document.SceneDocument {
	return s.doc
}

func (s *Session) Documents() *document.Manager {
	return s.edit
}

func (s *Session) items(nodes []ids.NodeID) []selection.Item {
	if s.doc == nil {
		return nil
	}
	out := make([]selection.Item, len(nodes))
	for i, n := range nodes {
		out[i] = selection.Item{Doc: s.doc.ID, Node: n}
	}
	return out
}

func (s *Session) Select(nodes ...ids.NodeID) {
	s.sel.Set(s.items(nodes)...)
}

func (s *Session) AddToSelection(nodes ...ids.NodeID) {
	s.sel.Add(s.items(nodes)...)
}

func (s *Session) ToggleSelection(n ids.NodeID) {
	if s.doc != nil {
		s.sel.Toggle(selection.Item{Doc: s.doc.ID, Node: n})
	}
}

func (s *Session) IsSelected(n ids.NodeID) bool {
	return s.doc != nil && s.sel.Contains(selection.Item{Doc: s.doc.ID, Node: n})
}

func (s *Session) Selected() []ids.NodeID {
	if s.doc == nil {
		s.scratch = s.scratch[:0]
		return s.scratch
	}
	s.scratch = s.sel.Nodes(s.doc.ID, s.scratch)
	return s.scratch
}

func (s *Session) SelectionState() *selection.Service {
	return s.sel
}

type Clipboard struct {
	text string
	mime string
	data []byte
}

func (c *Clipboard) Text() string {
	return c.text
}

func (c *Clipboard) SetText(t string) {
	*c = Clipboard{text: t}
}

func (c *Clipboard) SetData(mime string, data []byte, text string) bool {
	*c = Clipboard{text: text, mime: mime, data: bytes.Clone(data)}
	return true
}

func (c *Clipboard) Data(mime string) ([]byte, bool) {
	if mime == "" || mime != c.mime {
		return nil, false
	}
	return bytes.Clone(c.data), true
}

var errUnavailable = errs.New(errs.EditorCommand, "this is not available in the documentation; open the scene in the editor")

type Unavailable struct{}

func (Unavailable) AttachScriptType([]ids.NodeID, string) error {
	return errUnavailable
}

func (Unavailable) ScriptDir(string) string {
	return ""
}

func (Unavailable) CopyAssets([]ids.AssetID) bool {
	return false
}

func (Unavailable) ImportImage(string, string, image.Image) (ids.AssetID, string, error) {
	return ids.AssetID{}, "", errUnavailable
}

func (Unavailable) Create(ids.NodeID, templates.Place, *mathx.Vec2, func([]ids.NodeID)) {}

func (Unavailable) Instantiate(templates.Template, ids.NodeID, int) error {
	return errUnavailable
}

func (Unavailable) InstantiateAt(templates.Template, *mathx.Vec2) error {
	return errUnavailable
}

func (Unavailable) InstantiateAt3(templates.Template, mathx.Vec3) error {
	return errUnavailable
}

func (Unavailable) AttachScript([]ids.NodeID, ids.AssetID) {}

func (Unavailable) RenameScene(*document.SceneDocument) {}

func (Unavailable) SaveSceneAs(*document.SceneDocument) {}

var (
	_ services.Documents = (*Session)(nil)
	_ services.Selection = (*Session)(nil)
	_ services.AssetOps  = Unavailable{}
	_ services.SceneOps  = Unavailable{}
)
