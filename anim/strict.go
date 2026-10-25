package anim

import (
	"slices"
	"strconv"
	"strings"

	"aego/format/aetext"
)

// shape lists the attributes, fields and child blocks a block of a clip or controller may hold.
type shape struct {
	attrs  []string
	fields []string
	blocks []string
}

var (
	clipShape   = shape{attrs: []string{"version", "length", "fps", "loop"}, blocks: []string{"track", "frames", "event"}}
	trackShape  = shape{attrs: []string{"prop", "kind", "numeric", "root", "authority", "onEnd"}, blocks: []string{"key"}}
	keyShape    = shape{attrs: []string{"t", "v", "interp", "tangent", "in", "out"}}
	framesShape = shape{attrs: []string{"start", "fps", "prop", "flipProp", "authority"}, fields: []string{"sprites", "durations", "flipX"}}
	eventShape  = shape{attrs: []string{"t", "name", "payload", "mode"}}
	ctrlShape   = shape{attrs: []string{"version", "ease", "entryPos", "anyPos"}, fields: []string{"entry"}, blocks: []string{"param", "state", "transition"}}
	paramShape  = shape{attrs: []string{"kind", "default"}}
	stateShape  = shape{attrs: []string{"clip", "speed", "loop", "pos", "events"}}
	transShape  = shape{attrs: []string{"to", "duration", "exitTime", "interrupt"}, blocks: []string{"cond"}}
	condShape   = shape{attrs: []string{"op", "value"}}
)

// opening is how the block's first line reads with the attribute moved into it.
func opening(n *aetext.Node, f aetext.Field) string {
	var b strings.Builder
	b.WriteString(n.Head)
	if n.Label != "" || n.Head == "frames" || n.Head == "track" {
		b.WriteString(" " + strconv.Quote(n.Label))
	}
	b.WriteString(" " + f.Name + "=" + aetext.FormatValue(f.Value) + " {")
	return b.String()
}

// strict refuses names a block does not know, so a misspelt or misplaced setting is never dropped silently.
func strict(n *aetext.Node, s shape) error {
	for _, f := range n.Attrs {
		switch {
		case slices.Contains(s.attrs, f.Name):
		case slices.Contains(s.fields, f.Name):
			return textErr(n, f.Name+" is a field of "+n.Head+": write "+f.Name+"="+aetext.FormatValue(f.Value)+" inside the block")
		default:
			return textErr(n, "unknown attribute "+f.Name+" of "+n.Head+"; it takes "+strings.Join(s.attrs, ", "))
		}
	}
	for _, f := range n.Fields {
		switch {
		case slices.Contains(s.fields, f.Name):
		case slices.Contains(s.attrs, f.Name):
			return textErr(n, f.Name+" is an attribute of "+n.Head+": write `"+opening(n, f)+"`")
		case len(s.fields) == 0:
			return textErr(n, "unknown field "+f.Name+": "+n.Head+" takes no fields")
		default:
			return textErr(n, "unknown field "+f.Name+" of "+n.Head+"; it takes "+strings.Join(s.fields, ", "))
		}
	}
	for _, b := range n.Blocks {
		if !slices.Contains(s.blocks, b.Head) {
			if len(s.blocks) == 0 {
				return textErr(b, n.Head+" holds no blocks, found "+b.Head)
			}
			return textErr(b, "unknown block "+b.Head+" in "+n.Head+"; it holds "+strings.Join(s.blocks, ", "))
		}
	}
	return nil
}
