package asset

import (
	"math"
	"slices"
	"strings"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/format/aetext"
)

const MetaVersion = 1

const MetaSuffix = ".meta"

type ImportSettings map[string]aetext.Value

func (s ImportSettings) Clone() ImportSettings {
	out := make(ImportSettings, len(s))
	for k, v := range s {
		out[k] = v.Clone()
	}
	return out
}

func (s ImportSettings) String(key, def string) string {
	if v, ok := s[key]; ok && v.Kind == aetext.VString {
		return v.Str
	}
	if v, ok := s[key]; ok && v.Kind == aetext.VIdent {
		return v.Ident
	}
	return def
}

func (s ImportSettings) Bool(key string, def bool) bool {
	if v, ok := s[key]; ok && v.Kind == aetext.VBool {
		return v.Bool
	}
	return def
}

func (s ImportSettings) Int(key string, def int) int {
	if v, ok := s[key]; ok && (v.Kind == aetext.VInt || v.Kind == aetext.VFloat) {
		return int(v.AsInt())
	}
	return def
}

func (s ImportSettings) Float(key string, def float64) float64 {
	if v, ok := s[key]; ok && (v.Kind == aetext.VInt || v.Kind == aetext.VFloat) {
		return v.AsFloat()
	}
	return def
}

// SubAsset is one cell of a sliced sheet: its own id and a pixel rectangle of the sheet (D339).
type SourceFrame struct {
	Name string
	ID   ids.AssetID
	Rect [4]int
	// Pivot is normalized in the original logical canvas; absent means center.
	// Node SpriteRenderer.Origin remains authoritative for existing scenes.
	Pivot    [2]float32
	HasPivot bool
	unknown  *aetext.Node
}

type SourceMeta struct {
	ID       ids.AssetID
	Importer string
	Version  int
	Settings ImportSettings
	Subs     []SourceFrame
	Pivot    [2]float32
	HasPivot bool
	unknown  *aetext.Node
}

func NewSourceMeta(importer string, version int, defaults ImportSettings) *SourceMeta {
	return &SourceMeta{
		ID:       ids.NewAssetID(),
		Importer: importer,
		Version:  version,
		Settings: defaults.Clone(),
	}
}

func ParseSourceMeta(src []byte, name string) (*SourceMeta, error) {
	doc, err := aetext.Parse(src, name)
	if err != nil {
		return nil, err
	}
	if doc.Head != "meta" {
		return nil, errs.New(errs.AssetBadMeta, "not a meta file", log.F("head", doc.Head), log.F("path", name))
	}
	m := &SourceMeta{Settings: ImportSettings{}, unknown: doc.Clone()}
	if p, present, err := parseSourcePivot(doc); err != nil {
		return nil, err
	} else {
		m.Pivot, m.HasPivot = p, present
	}
	if v, ok := doc.Attr("version"); ok && int(v.AsInt()) > MetaVersion {
		return nil, errs.New(errs.AssetBadMeta, "meta was written by a newer engine",
			log.F("path", name), log.F("version", v.AsInt()))
	}
	v, ok := doc.Field("id")
	if !ok {
		return nil, errs.New(errs.AssetBadMeta, "meta has no id", log.F("path", name))
	}
	id, err := ids.ParseAssetID(v.Str)
	if err != nil {
		return nil, err
	}
	m.ID = id
	if v, ok := doc.Field("importer"); ok {
		m.Importer = v.Str
	}
	if v, ok := doc.Field("importerVersion"); ok {
		m.Version = int(v.AsInt())
	}
	if s := doc.Block("settings", ""); s != nil {
		for _, f := range s.Fields {
			m.Settings[f.Name] = f.Value
		}
	}
	var bad error
	doc.EachBlock("sub", func(n *aetext.Node) {
		sa := SourceFrame{Name: n.Label, unknown: n.Clone()}
		if p, present, err := parseSourcePivot(n); err != nil {
			bad = err
			return
		} else {
			sa.Pivot, sa.HasPivot = p, present
		}
		v, _ := n.Attr("id")
		id, err := ids.ParseAssetID(v.Str)
		if err != nil || sa.Name == "" {
			bad = errs.New(errs.AssetBadMeta, "a sub-asset needs a name and an id", log.F("path", name), log.F("sub", sa.Name))
			return
		}
		sa.ID = id
		if r, ok := n.Attr("rect"); ok && len(r.List) == 4 {
			for i, x := range r.List {
				sa.Rect[i] = int(x.AsInt())
			}
		}
		m.Subs = append(m.Subs, sa)
	})
	if bad != nil {
		return nil, bad
	}
	return m, nil
}

func MarshalSourceMeta(m *SourceMeta) []byte {
	root := m.unknown.Clone()
	if root == nil {
		root = &aetext.Node{Head: "meta"}
	}
	root.Head = "meta"
	setSourceField(&root.Attrs, "version", aetext.Int(MetaVersion))
	setSourceField(&root.Fields, "id", aetext.String(m.ID.String()))
	setSourceField(&root.Fields, "importer", aetext.String(m.Importer))
	setSourceField(&root.Fields, "importerVersion", aetext.Int(int64(m.Version)))
	writeSourcePivot(root, m.Pivot, m.HasPivot)
	settings := root.Block("settings", "").Clone()
	if settings == nil {
		settings = &aetext.Node{Head: "settings"}
	}
	settings.Fields = nil
	for _, key := range sourceSettingKeys(m.Settings) {
		settings.Fields = append(settings.Fields, aetext.Field{Name: key, Value: sourceSettingValue(m.Settings[key])})
	}
	var blocks []*aetext.Node
	for _, b := range root.Blocks {
		if b.Head != "settings" && b.Head != "sub" {
			blocks = append(blocks, b)
		}
	}
	root.Blocks = append([]*aetext.Node{settings}, sourceFrameBlocks(m.Subs)...)
	root.Blocks = append(root.Blocks, blocks...)
	return aetext.Write(root)
}

func sourceFrameBlocks(subs []SourceFrame) []*aetext.Node {
	out := make([]*aetext.Node, 0, len(subs))
	// Source order is authoring data. IDs, not names or slice indices, own links.
	for _, sa := range subs {
		n := sa.unknown.Clone()
		if n == nil {
			n = &aetext.Node{Head: "sub"}
		}
		n.Head, n.Label = "sub", sa.Name
		setSourceField(&n.Attrs, "id", aetext.String(sa.ID.String()))
		r := sa.Rect
		setSourceField(&n.Attrs, "rect", aetext.List(aetext.Int(int64(r[0])), aetext.Int(int64(r[1])), aetext.Int(int64(r[2])), aetext.Int(int64(r[3]))))
		writeSourcePivot(n, sa.Pivot, sa.HasPivot)
		out = append(out, n)
	}
	return out
}

func MarshalSourceFrames(subs []SourceFrame) []byte {
	return aetext.Write(&aetext.Node{Head: "subs", Blocks: sourceFrameBlocks(subs)})
}
func MarshalImportSettings(s ImportSettings) []byte {
	n := &aetext.Node{Head: "settings"}
	for _, key := range sourceSettingKeys(s) {
		n.Fields = append(n.Fields, aetext.Field{Name: key, Value: sourceSettingValue(s[key])})
	}
	return aetext.Write(n)
}
func sourceSettingKeys(s ImportSettings) []string {
	keys := make([]string, 0, len(s))
	for key := range s {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
func setSourceField(fields *[]aetext.Field, name string, value aetext.Value) {
	for i := range *fields {
		if (*fields)[i].Name == name {
			(*fields)[i].Value = value
			return
		}
	}
	*fields = append(*fields, aetext.Field{Name: name, Value: value})
}
func writeSourcePivot(n *aetext.Node, p [2]float32, present bool) {
	n.Attrs = slices.DeleteFunc(n.Attrs, func(f aetext.Field) bool { return f.Name == "pivot" })
	n.Fields = slices.DeleteFunc(n.Fields, func(f aetext.Field) bool { return f.Name == "pivot" })
	if present {
		setSourceField(&n.Attrs, "pivot", aetext.List(aetext.Float32(p[0]), aetext.Float32(p[1])))
	}
}
func parseSourcePivot(n *aetext.Node) ([2]float32, bool, error) {
	v, ok := n.Attr("pivot")
	if !ok {
		v, ok = n.Field("pivot")
	}
	if !ok {
		return [2]float32{}, false, nil
	}
	if v.Kind != aetext.VList || len(v.List) != 2 {
		return [2]float32{}, true, errs.New(errs.AssetBadMeta, "pivot needs two finite normalized coordinates")
	}
	var p [2]float32
	for i, x := range v.List {
		f := x.AsFloat()
		if (x.Kind != aetext.VFloat && x.Kind != aetext.VInt) || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 || f > 1 {
			return p, true, errs.New(errs.AssetBadMeta, "pivot coordinates must be in [0,1]")
		}
		p[i] = float32(f)
	}
	return p, true, nil
}
func (f SourceFrame) EffectivePivot() [2]float32 {
	if f.HasPivot {
		return f.Pivot
	}
	return [2]float32{.5, .5}
}
func (m *SourceMeta) EffectivePivot() [2]float32 {
	if m.HasPivot {
		return m.Pivot
	}
	return [2]float32{.5, .5}
}
func (m *SourceMeta) Frame(id ids.AssetID) (*SourceFrame, bool) {
	for i := range m.Subs {
		if m.Subs[i].ID == id {
			return &m.Subs[i], true
		}
	}
	return nil, false
}

// Enum labels with separators are strings in Aetext, never malformed identifiers.
func sourceSettingValue(v aetext.Value) aetext.Value {
	if v.Kind == aetext.VIdent && strings.Contains(v.Ident, "-") {
		return aetext.String(v.Ident)
	}
	return v.Clone()
}
