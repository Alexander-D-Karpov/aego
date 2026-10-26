package art

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"aego/core/errs"
	"aego/engine"
	"aego/engine/module"
	"aego/format/aetext"
)

// ID is the module and its project.aego block's art { palette pixelsPerUnit }: the look a project's drawings share, read by agents and Project Health (D363).
const (
	ID        = "art"
	SectionID = ID
)

// Art is the shared style: the palette sprites are drawn in and how many art pixels make a world unit.
type Art struct {
	Palette []color.NRGBA
	// PixelsPerUnit is art pixels per world unit; a drawing is scaled up to the render pixelsPerUnit. Zero means the same.
	PixelsPerUnit int32
}

// Hex writes a palette color as #rrggbb.
func Hex(c color.NRGBA) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

// ParseHex reads #rgb or #rrggbb.
func ParseHex(s string) (color.NRGBA, error) {
	h := strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	n, err := strconv.ParseUint(h, 16, 32)
	if len(h) != 6 || err != nil {
		return color.NRGBA{}, errs.New(errs.ProjectBadFile, "art: "+strconv.Quote(s)+" is not a color like #d0312d")
	}
	return color.NRGBA{uint8(n >> 16), uint8(n >> 8), uint8(n), 255}, nil
}

func decodeArt(n *aetext.Node, a *Art) error {
	for _, f := range n.Fields {
		switch f.Name {
		case "palette":
			for _, v := range f.Value.List {
				var c color.NRGBA
				switch v.Kind {
				case aetext.VColor:
					c = color.NRGBA{uint8(v.Color >> 24), uint8(v.Color >> 16), uint8(v.Color >> 8), 255}
				default:
					var err error
					if c, err = ParseHex(v.Str); err != nil {
						return err
					}
				}
				a.Palette = append(a.Palette, c)
			}
		case "pixelsPerUnit":
			a.PixelsPerUnit = int32(f.Value.AsInt())
		}
	}
	return nil
}

func encodeArt(a *Art, n *aetext.Node) {
	if len(a.Palette) > 0 {
		items := make([]aetext.Value, len(a.Palette))
		for i, c := range a.Palette {
			items[i] = aetext.String(Hex(c))
		}
		n.Fields = append(n.Fields, aetext.Field{Name: "palette", Value: aetext.List(items...)})
	}
	if a.PixelsPerUnit != 0 {
		n.Fields = append(n.Fields, aetext.Field{Name: "pixelsPerUnit", Value: aetext.Int(int64(a.PixelsPerUnit))})
	}
}

func validateArt(a *Art) error {
	if a.PixelsPerUnit < 0 {
		return errs.New(errs.ProjectBadFile, "art: pixelsPerUnit must not be negative")
	}
	if len(a.Palette) > 64 {
		return errs.New(errs.ProjectBadFile, "art: a palette has at most 64 colors")
	}
	return nil
}

// Scale is how many texture pixels one art pixel becomes so drawings match renderPPU; at least 1.
func (a Art) Scale(renderPPU float32) int {
	if a.PixelsPerUnit <= 0 {
		return 1
	}
	return max(1, int(renderPPU)/int(a.PixelsPerUnit))
}

// Module owns the art section; it has no runtime part.
type Module struct{}

func New() *Module { return &Module{} }

func (*Module) Info() module.Info {
	return module.Info{ID: ID, Version: engine.Version, Builtin: true, DisplayName: "Art", DocsOrder: 116, DocsIcon: "file.texture"}
}

func (*Module) Register(r *module.Registry) error {
	r.ProjectSection(module.SectionDesc[Art]{ID: SectionID, Title: "Art", Order: 110,
		Default: func() Art { return Art{} }, Decode: decodeArt, Encode: encodeArt, Validate: validateArt})
	return nil
}

func (*Module) Init(*module.InitContext) error { return nil }

func (*Module) Shutdown() {}
