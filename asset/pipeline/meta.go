package pipeline

import (
	"aego/asset"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/format/aetext"
)

const MetaVersion = 1

const MetaSuffix = ".meta"

type Settings map[string]aetext.Value

func (s Settings) Clone() Settings {
	out := make(Settings, len(s))
	for k, v := range s {
		out[k] = v
	}
	return out
}

func (s Settings) String(key, def string) string {
	if v, ok := s[key]; ok && v.Kind == aetext.VString {
		return v.Str
	}
	if v, ok := s[key]; ok && v.Kind == aetext.VIdent {
		return v.Ident
	}
	return def
}

func (s Settings) Bool(key string, def bool) bool {
	if v, ok := s[key]; ok && v.Kind == aetext.VBool {
		return v.Bool
	}
	return def
}

func (s Settings) Int(key string, def int) int {
	if v, ok := s[key]; ok && (v.Kind == aetext.VInt || v.Kind == aetext.VFloat) {
		return int(v.AsInt())
	}
	return def
}

type Meta struct {
	ID       ids.AssetID
	Importer string
	Version  int
	Settings Settings
}

func NewMeta(importer string, version int, defaults Settings) *Meta {
	return &Meta{
		ID:       ids.NewAssetID(),
		Importer: importer,
		Version:  version,
		Settings: defaults.Clone(),
	}
}

func ParseMeta(src []byte, name string) (*Meta, error) {
	doc, err := aetext.Parse(src, name)
	if err != nil {
		return nil, err
	}
	if doc.Head != "meta" {
		return nil, errs.New(errs.AssetBadMeta, "not a meta file", log.F("head", doc.Head), log.F("path", name))
	}
	m := &Meta{Settings: Settings{}}
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
	return m, nil
}

func WriteMeta(m *Meta) []byte {
	root := &aetext.Node{Head: "meta"}
	root.Attrs = append(root.Attrs, aetext.Field{Name: "version", Value: aetext.Int(MetaVersion)})
	root.Fields = append(root.Fields,
		aetext.Field{Name: "id", Value: aetext.String(m.ID.String())},
		aetext.Field{Name: "importer", Value: aetext.String(m.Importer)},
		aetext.Field{Name: "importerVersion", Value: aetext.Int(int64(m.Version))},
	)
	settings := &aetext.Node{Head: "settings"}
	for _, key := range sortedKeys(m.Settings) {
		settings.Fields = append(settings.Fields, aetext.Field{Name: key, Value: m.Settings[key]})
	}
	root.Blocks = append(root.Blocks, settings)
	return aetext.Write(root)
}

func canonicalSettings(s Settings) []byte {
	n := &aetext.Node{Head: "settings"}
	for _, key := range sortedKeys(s) {
		n.Fields = append(n.Fields, aetext.Field{Name: key, Value: s[key]})
	}
	return aetext.Write(n)
}

var _ = asset.ArtTexture
