package bundle

import (
	"strconv"

	"aego/asset"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/format/aetext"
)

const (
	DataDir      = "data"
	ManifestFile = "manifest.aetext"
	ExportFile   = "export.aetext"
	Scheme       = "data"
	ArtifactsDir = "artifacts"
	Version      = 1
	Release      = "release"
	Development  = "development"
)

var CoreBlocks = []string{"window", "defaults"}

type Window struct {
	Width  int
	Height int
	Title  string
}

type Game struct {
	Name      string
	Module    string
	Version   string
	MainScene ids.AssetID
	Window    Window
	Defaults  *aetext.Node
	Blocks    []*aetext.Node
}

func (g *Game) Block(head string) *aetext.Node {
	for _, b := range g.Blocks {
		if b.Head == head {
			return b
		}
	}
	return nil
}

type Data struct {
	Engine  string
	Profile string
	Target  string
	Game    Game
	Index   *asset.Index
}

func (d *Data) Release() bool {
	return d.Profile == Release
}

func ArtifactPath(name string) string {
	return Scheme + "://" + ArtifactsDir + "/" + name
}

type Lib struct {
	Name    string `json:"name"`
	Link    string `json:"link"`
	File    string `json:"file,omitempty"`
	License string `json:"license,omitempty"`
}

type Size struct {
	Kind  string `json:"kind"`
	Count int    `json:"count"`
	Bytes int64  `json:"bytes"`
}

type Export struct {
	Engine      string
	Game        string
	GameVersion string
	Module      string
	Target      string
	Profile     string
	Binary      string
	BinarySize  int64
	StartScene  ids.AssetID
	Modules     []string
	Libs        []Lib
	Data        []Size
	DataSize    int64
	Artifacts   int
	Hash        string
	BuildMillis int64
	Time        string
}

func text(name, v string) aetext.Field {
	return aetext.Field{Name: name, Value: aetext.String(v)}
}

func integer(name string, v int64) aetext.Field {
	return aetext.Field{Name: name, Value: aetext.Int(v)}
}

func strs(name string, list []string) aetext.Field {
	vs := make([]aetext.Value, len(list))
	for i, s := range list {
		vs[i] = aetext.String(s)
	}
	return aetext.Field{Name: name, Value: aetext.List(vs...)}
}

func getStr(n *aetext.Node, name string) string {
	if v, ok := n.Field(name); ok {
		return v.Str
	}
	return ""
}

func getInt(n *aetext.Node, name string) int64 {
	if v, ok := n.Field(name); ok {
		return v.AsInt()
	}
	return 0
}

func getStrs(n *aetext.Node, name string) []string {
	v, ok := n.Field(name)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(v.List))
	for _, x := range v.List {
		out = append(out, x.Str)
	}
	return out
}

func getID(n *aetext.Node, name, file string) (ids.AssetID, error) {
	s := getStr(n, name)
	if s == "" {
		return ids.AssetID{}, nil
	}
	id, err := ids.ParseAssetID(s)
	if err != nil {
		return ids.AssetID{}, errs.Wrap(err, errs.AssetBadIndex, name+" is not an asset id").With(log.F("file", file))
	}
	return id, nil
}

func header(doc *aetext.Node, head, file string) error {
	if doc.Head != head {
		return errs.New(errs.AssetBadIndex, "not a "+head+" file", log.F("file", file), log.F("head", doc.Head))
	}
	if v, ok := doc.Attr("version"); ok && int(v.AsInt()) > Version {
		return errs.New(errs.AssetBadIndex, file+" was written by a newer engine",
			log.F("version", v.AsInt()), log.F("supported", Version))
	}
	return nil
}

func EncodeData(d *Data) ([]byte, error) {
	root := &aetext.Node{Head: "manifest", Attrs: []aetext.Field{integer("version", Version)}}
	root.Fields = append(root.Fields, text("engine", d.Engine), text("profile", d.Profile), text("target", d.Target))
	g := &aetext.Node{Head: "game"}
	g.Fields = append(g.Fields,
		text("name", d.Game.Name),
		text("module", d.Game.Module),
		text("version", d.Game.Version),
		aetext.Field{Name: "mainScene", Value: aetext.AssetRef(d.Game.MainScene.String())},
	)
	win := &aetext.Node{Head: "window"}
	win.Fields = append(win.Fields,
		integer("width", int64(d.Game.Window.Width)),
		integer("height", int64(d.Game.Window.Height)),
		text("title", d.Game.Window.Title),
	)
	g.Blocks = append(g.Blocks, win)
	if d.Game.Defaults != nil {
		def := d.Game.Defaults.Clone()
		def.Head = "defaults"
		g.Blocks = append(g.Blocks, def)
	}
	g.Blocks = append(g.Blocks, d.Game.Blocks...)
	root.Blocks = append(root.Blocks, g)
	idx, err := aetext.Parse(asset.WriteIndex(d.Index), "index")
	if err != nil {
		return nil, err
	}
	root.Blocks = append(root.Blocks, idx)
	return aetext.Write(root), nil
}

func DecodeData(src []byte, file string) (*Data, error) {
	doc, err := aetext.Parse(src, file)
	if err != nil {
		return nil, err
	}
	if err := header(doc, "manifest", file); err != nil {
		return nil, err
	}
	d := &Data{Engine: getStr(doc, "engine"), Profile: getStr(doc, "profile"), Target: getStr(doc, "target")}
	g := doc.Block("game", "")
	if g == nil {
		return nil, errs.New(errs.AssetBadIndex, "the manifest has no game block", log.F("file", file))
	}
	d.Game.Name, d.Game.Module, d.Game.Version = getStr(g, "name"), getStr(g, "module"), getStr(g, "version")
	if d.Game.MainScene, err = getID(g, "mainScene", file); err != nil {
		return nil, err
	}
	d.Game.Window = Window{Width: 1280, Height: 720, Title: d.Game.Name}
	for _, b := range g.Blocks {
		switch b.Head {
		case "window":
			if v := getInt(b, "width"); v > 0 {
				d.Game.Window.Width = int(v)
			}
			if v := getInt(b, "height"); v > 0 {
				d.Game.Window.Height = int(v)
			}
			if v := getStr(b, "title"); v != "" {
				d.Game.Window.Title = v
			}
		case "defaults":
			d.Game.Defaults = b
		default:
			d.Game.Blocks = append(d.Game.Blocks, b)
		}
	}
	ib := doc.Block("index", "")
	if ib == nil {
		return nil, errs.New(errs.AssetBadIndex, "the manifest has no asset index", log.F("file", file))
	}
	if d.Index, err = asset.ReadIndex(aetext.Write(ib), file); err != nil {
		return nil, err
	}
	return d, nil
}

func EncodeExport(x *Export) []byte {
	root := &aetext.Node{Head: "export", Attrs: []aetext.Field{integer("version", Version)}}
	root.Fields = append(root.Fields,
		text("engine", x.Engine),
		text("game", x.Game),
		text("gameVersion", x.GameVersion),
		text("module", x.Module),
		text("target", x.Target),
		text("profile", x.Profile),
		text("binary", x.Binary),
		integer("binarySize", x.BinarySize),
		aetext.Field{Name: "startScene", Value: aetext.AssetRef(x.StartScene.String())},
		strs("modules", x.Modules),
		integer("dataSize", x.DataSize),
		integer("artifacts", int64(x.Artifacts)),
		text("hash", x.Hash),
		integer("buildMillis", x.BuildMillis),
		text("time", x.Time),
	)
	for _, l := range x.Libs {
		n := &aetext.Node{Head: "lib", Label: l.Name}
		n.Fields = append(n.Fields, text("link", l.Link))
		if l.File != "" {
			n.Fields = append(n.Fields, text("file", l.File))
		}
		if l.License != "" {
			n.Fields = append(n.Fields, text("license", l.License))
		}
		root.Blocks = append(root.Blocks, n)
	}
	for _, s := range x.Data {
		root.Blocks = append(root.Blocks, &aetext.Node{Head: "data", Label: s.Kind,
			Fields: []aetext.Field{integer("count", int64(s.Count)), integer("bytes", s.Bytes)}})
	}
	return aetext.Write(root)
}

func DecodeExport(src []byte, file string) (*Export, error) {
	doc, err := aetext.Parse(src, file)
	if err != nil {
		return nil, err
	}
	if err := header(doc, "export", file); err != nil {
		return nil, err
	}
	x := &Export{
		Engine: getStr(doc, "engine"), Game: getStr(doc, "game"), GameVersion: getStr(doc, "gameVersion"),
		Module: getStr(doc, "module"), Target: getStr(doc, "target"), Profile: getStr(doc, "profile"),
		Binary: getStr(doc, "binary"), BinarySize: getInt(doc, "binarySize"), Modules: getStrs(doc, "modules"),
		DataSize: getInt(doc, "dataSize"), Artifacts: int(getInt(doc, "artifacts")), Hash: getStr(doc, "hash"),
		BuildMillis: getInt(doc, "buildMillis"), Time: getStr(doc, "time"),
	}
	if x.StartScene, err = getID(doc, "startScene", file); err != nil {
		return nil, err
	}
	doc.EachBlock("lib", func(n *aetext.Node) {
		x.Libs = append(x.Libs, Lib{Name: n.Label, Link: getStr(n, "link"), File: getStr(n, "file"), License: getStr(n, "license")})
	})
	doc.EachBlock("data", func(n *aetext.Node) {
		x.Data = append(x.Data, Size{Kind: n.Label, Count: int(getInt(n, "count")), Bytes: getInt(n, "bytes")})
	})
	return x, nil
}

func (s Size) String() string {
	return s.Kind + " " + strconv.Itoa(s.Count) + " " + strconv.FormatInt(s.Bytes, 10)
}
