package state

import (
	"os"
	"path/filepath"
	"sort"
	"time"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/document"
	"aego/format/aetext"
)

const (
	UserDir    = ".aego/user"
	EditorFile = "editor.aetext"
	LayoutFile = "layout.aetext"
	Version    = 1
)

type Doc struct {
	Path      string
	Camera    document.Camera
	Mode      int
	Orbit     document.Orbit
	Expanded  []ids.NodeID
	Selection []ids.NodeID
}

type Editor struct {
	Docs            []Doc
	Active          string
	Tool            int
	Grid            bool
	Snap            bool
	SnapMove        float32
	SnapRotate      float32
	SnapScale       float32
	WindowW         int
	WindowH         int
	Collapsed       []string
	Autosave        bool
	InlayHints      bool
	Minimap         bool
	RainbowBrackets bool
	HighlightBlock  bool
}

func Default() *Editor {
	return &Editor{Grid: true, SnapMove: 0.5, SnapRotate: 15, SnapScale: 0.1, Autosave: true, InlayHints: true, Minimap: true, RainbowBrackets: true, HighlightBlock: true}
}

func userPath(root, name string) string {
	return filepath.Join(root, filepath.FromSlash(UserDir), name)
}

func WriteTrace(root string, data []byte) (string, error) {
	p := userPath(root, filepath.Join("traces", "trace-"+time.Now().Format("20060102-150405")+".json"))
	return p, document.WriteAtomic(p, data)
}

func Load(root string) (*Editor, error) {
	data, err := os.ReadFile(userPath(root, EditorFile))
	if os.IsNotExist(err) {
		return Default(), nil
	}
	if err != nil {
		return Default(), errs.Wrap(err, errs.EditorDocument, "cannot read the editor state")
	}
	return Decode(data, EditorFile)
}

func Save(root string, s *Editor) error {
	return document.WriteAtomic(userPath(root, EditorFile), Encode(s))
}

func LoadLayout(root string) ([]byte, bool) {
	data, err := os.ReadFile(userPath(root, LayoutFile))
	return data, err == nil
}

func SaveLayout(root string, data []byte) error {
	return document.WriteAtomic(userPath(root, LayoutFile), data)
}

func RemoveLayout(root string) error {
	err := os.Remove(userPath(root, LayoutFile))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func SortNodes(list []ids.NodeID) {
	sort.Slice(list, func(i, j int) bool {
		if list[i].Hi != list[j].Hi {
			return list[i].Hi < list[j].Hi
		}
		return list[i].Lo < list[j].Lo
	})
}

func nodeList(list []ids.NodeID) aetext.Value {
	items := make([]aetext.Value, len(list))
	for i, id := range list {
		items[i] = aetext.NodeRef(id.String())
	}
	return aetext.List(items...)
}

func readNodes(v aetext.Value) []ids.NodeID {
	var out []ids.NodeID
	for _, item := range v.List {
		if id, err := ids.ParseNodeID(item.Str); err == nil {
			out = append(out, id)
		}
	}
	return out
}

func Encode(s *Editor) []byte {
	root := &aetext.Node{Head: "editor"}
	root.Attrs = append(root.Attrs, aetext.Field{Name: "version", Value: aetext.Int(Version)})
	root.Fields = append(root.Fields,
		aetext.Field{Name: "active", Value: aetext.String(s.Active)},
		aetext.Field{Name: "tool", Value: aetext.Int(int64(s.Tool))},
		aetext.Field{Name: "grid", Value: aetext.Bool(s.Grid)},
		aetext.Field{Name: "snap", Value: aetext.Bool(s.Snap)},
		aetext.Field{Name: "snapMove", Value: aetext.Float32(s.SnapMove)},
		aetext.Field{Name: "snapRotate", Value: aetext.Float32(s.SnapRotate)},
		aetext.Field{Name: "snapScale", Value: aetext.Float32(s.SnapScale)},
		aetext.Field{Name: "window", Value: aetext.List(aetext.Int(int64(s.WindowW)), aetext.Int(int64(s.WindowH)))},
		aetext.Field{Name: "autosave", Value: aetext.Bool(s.Autosave)},
		aetext.Field{Name: "inlayHints", Value: aetext.Bool(s.InlayHints)},
		aetext.Field{Name: "minimap", Value: aetext.Bool(s.Minimap)},
		aetext.Field{Name: "codeRainbow", Value: aetext.Bool(s.RainbowBrackets)},
		aetext.Field{Name: "highlightBlock", Value: aetext.Bool(s.HighlightBlock)},
	)
	collapsed := make([]aetext.Value, len(s.Collapsed))
	for i, k := range s.Collapsed {
		collapsed[i] = aetext.String(k)
	}
	root.Fields = append(root.Fields, aetext.Field{Name: "collapsed", Value: aetext.List(collapsed...)})
	for _, d := range s.Docs {
		o := d.Orbit
		n := &aetext.Node{Head: "doc", Label: d.Path}
		n.Fields = append(n.Fields,
			aetext.Field{Name: "camera", Value: aetext.List(aetext.Float32(d.Camera.Position.X), aetext.Float32(d.Camera.Position.Y))},
			aetext.Field{Name: "zoom", Value: aetext.Float32(d.Camera.Zoom)},
			aetext.Field{Name: "rotation", Value: aetext.Float32(d.Camera.Rotation)},
			aetext.Field{Name: "mode", Value: aetext.Int(int64(d.Mode))},
			aetext.Field{Name: "orbit", Value: aetext.List(
				aetext.Float32(o.Target.X), aetext.Float32(o.Target.Y), aetext.Float32(o.Target.Z),
				aetext.Float32(o.Distance), aetext.Float32(o.Yaw), aetext.Float32(o.Pitch), aetext.Float32(o.FOV),
			)},
			aetext.Field{Name: "expanded", Value: nodeList(d.Expanded)},
			aetext.Field{Name: "selection", Value: nodeList(d.Selection)},
		)
		if o.Ortho {
			n.Fields = append(n.Fields, aetext.Field{Name: "ortho", Value: aetext.Bool(true)})
		}
		root.Blocks = append(root.Blocks, n)
	}
	return aetext.Write(root)
}

func Decode(data []byte, name string) (*Editor, error) {
	doc, err := aetext.Parse(data, name)
	if err != nil {
		return Default(), err
	}
	if doc.Head != "editor" {
		return Default(), errs.New(errs.EditorDocument, "not an editor state file", log.F("head", doc.Head))
	}
	if v, ok := doc.Attr("version"); ok && int(v.AsInt()) > Version {
		return Default(), errs.New(errs.EditorDocument, "editor state was written by a newer editor")
	}
	s := Default()
	if v, ok := doc.Field("active"); ok {
		s.Active = v.Str
	}
	if v, ok := doc.Field("tool"); ok {
		s.Tool = int(v.AsInt())
	}
	if v, ok := doc.Field("grid"); ok {
		s.Grid = v.Bool
	}
	if v, ok := doc.Field("snap"); ok {
		s.Snap = v.Bool
	}
	if v, ok := doc.Field("snapMove"); ok {
		s.SnapMove = float32(v.AsFloat())
	}
	if v, ok := doc.Field("snapRotate"); ok {
		s.SnapRotate = float32(v.AsFloat())
	}
	if v, ok := doc.Field("snapScale"); ok {
		s.SnapScale = float32(v.AsFloat())
	}
	if v, ok := doc.Field("window"); ok && len(v.List) == 2 {
		s.WindowW, s.WindowH = int(v.List[0].AsInt()), int(v.List[1].AsInt())
	}
	if v, ok := doc.Field("autosave"); ok {
		s.Autosave = v.Bool
	}
	if v, ok := doc.Field("inlayHints"); ok {
		s.InlayHints = v.Bool
	}
	if v, ok := doc.Field("minimap"); ok {
		s.Minimap = v.Bool
	}
	if v, ok := doc.Field("codeRainbow"); ok {
		s.RainbowBrackets = v.Bool
	}
	if v, ok := doc.Field("highlightBlock"); ok {
		s.HighlightBlock = v.Bool
	}
	if v, ok := doc.Field("collapsed"); ok {
		for _, item := range v.List {
			s.Collapsed = append(s.Collapsed, item.Str)
		}
	}
	doc.EachBlock("doc", func(n *aetext.Node) {
		d := Doc{Path: n.Label, Camera: document.Camera{Zoom: 64}}
		if v, ok := n.Field("camera"); ok && len(v.List) == 2 {
			d.Camera.Position = mathx.Vec2{X: float32(v.List[0].AsFloat()), Y: float32(v.List[1].AsFloat())}
		}
		if v, ok := n.Field("zoom"); ok && v.AsFloat() > 0 {
			d.Camera.Zoom = float32(v.AsFloat())
		}
		if v, ok := n.Field("rotation"); ok {
			d.Camera.Rotation = float32(v.AsFloat())
		}
		if v, ok := n.Field("mode"); ok {
			d.Mode = int(v.AsInt())
		}
		if v, ok := n.Field("orbit"); ok && len(v.List) == 7 {
			f := func(i int) float32 { return float32(v.List[i].AsFloat()) }
			d.Orbit = document.Orbit{
				Target:   mathx.Vec3{X: f(0), Y: f(1), Z: f(2)},
				Distance: f(3), Yaw: f(4), Pitch: f(5), FOV: f(6),
			}
		}
		if v, ok := n.Field("ortho"); ok {
			d.Orbit.Ortho = v.Bool
		}
		if v, ok := n.Field("expanded"); ok {
			d.Expanded = readNodes(v)
		}
		if v, ok := n.Field("selection"); ok {
			d.Selection = readNodes(v)
		}
		if d.Path != "" {
			s.Docs = append(s.Docs, d)
		}
	})
	return s, nil
}
