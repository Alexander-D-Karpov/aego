package agentbridge

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"aego/app/agent"
	"aego/app/agent/mcp"
	"aego/scene"
	"aego/scene/scriptcomp"
)

var errNoScene = errors.New("no scene is open")

type sceneScript struct {
	Type string `json:"type"`
	Path string `json:"path,omitempty"`
	Line int    `json:"line,omitempty"`
}

type contextNode struct {
	ID      string        `json:"id"`
	Name    string        `json:"name"`
	Parent  string        `json:"parent,omitempty"`
	Scripts []sceneScript `json:"scripts,omitempty"`
}

type sceneContext struct {
	Binding   string            `json:"binding"`
	Document  string            `json:"document"`
	Revision  uint64            `json:"revision"`
	Dirty     bool              `json:"dirty"`
	Play      bool              `json:"play"`
	Root      string            `json:"root,omitempty"`
	Selected  []contextNode     `json:"selected,omitempty"`
	Nodes     []contextNode     `json:"nodes,omitempty"`
	Truncated bool              `json:"truncated,omitempty"`
	Sources   map[string]string `json:"scriptExcerpts,omitempty"`
}

func (b *Bridge) sceneContext(details bool) sceneContext {
	out := sceneContext{Binding: b.binding}
	d := b.ctx.Doc()
	if d == nil {
		return out
	}
	s := d.Scene()
	out.Document, out.Revision, out.Dirty, out.Play = d.Path, s.Revision(), d.Dirty(), b.ctx.Docs.InPlay()
	if !details {
		return out
	}
	root := s.Root()
	out.Root = s.Name(root)
	ref := func(h scene.NodeHandle) contextNode {
		n := contextNode{ID: s.ID(h).String(), Name: s.Name(h)}
		if parent := s.Parent(h); !parent.IsZero() {
			n.Parent = s.ID(parent).String()
		}
		if c, ok := scriptcomp.ScriptsOf(s, h); ok {
			for _, a := range c.Attachments {
				if len(n.Scripts) >= 8 {
					out.Truncated = true
					break
				}
				ref := sceneScript{Type: a.TypeName}
				if b.ctx.Code != nil {
					if l, found := b.ctx.Code.Declaration(a.TypeName, ""); found {
						ref.Path, ref.Line = l.Path, l.Line
					}
				}
				n.Scripts = append(n.Scripts, ref)
			}
		}
		return n
	}
	if b.ctx.Selection != nil {
		for _, id := range b.ctx.Selected() {
			if len(out.Selected) >= 8 {
				out.Truncated = true
				break
			}
			if h := s.Handle(id); s.IsAlive(h) {
				out.Selected = append(out.Selected, ref(h))
			}
		}
	}
	var walk func(scene.NodeHandle)
	walk = func(h scene.NodeHandle) {
		out.Nodes = append(out.Nodes, ref(h))
		s.Children(h, func(child scene.NodeHandle) bool {
			if len(out.Nodes) >= 40 {
				out.Truncated = true
				return false
			}
			walk(child)
			return true
		})
	}
	walk(root)
	for _, nodes := range [][]contextNode{out.Selected, out.Nodes} {
		for _, n := range nodes {
			for _, ref := range n.Scripts {
				if ref.Path == "" || len(out.Sources) >= 3 {
					continue
				}
				if _, ok := out.Sources[ref.Path]; ok {
					continue
				}
				if o, err := agent.Run(b.sess, "script.read", agent.Args{"file": ref.Path, "from": "1", "to": "36"}); err == nil && o != nil {
					text := o.Text
					if len(text) > 1800 {
						text = text[:1800]
						for !utf8.ValidString(text) {
							text = text[:len(text)-1]
						}
					}
					if out.Sources == nil {
						out.Sources = map[string]string{}
					}
					out.Sources[ref.Path] = strings.TrimSpace(text)
				}
			}
		}
	}
	return out
}

func (b *Bridge) SceneSnapshot() (string, error) {
	v := b.sceneContext(true)
	if v.Document == "" {
		return "", errNoScene
	}
	data, err := json.Marshal(v)
	return string(data), err
}

func (b *Bridge) contextTool() mcp.Tool {
	return mcp.Tool{Name: "editor_scene_context", ReadOnly: true, Description: "Read the current editor binding, open scene, revision and live selection; with details, include hierarchy and attached script sources, including unsaved changes.", Schema: map[string]any{"type": "object", "properties": map[string]any{"details": map[string]any{"type": "boolean"}}}, Run: func(args map[string]any) (string, bool) {
		var snap sceneContext
		generation := b.panelGeneration.Load()
		var stale bool
		details, _ := args["details"].(bool)
		if err := b.main(func() {
			stale = b.cancelled(generation)
			if !stale {
				snap = b.sceneContext(details)
			}
		}); err != nil {
			return err.Error(), true
		}
		if stale {
			return errCancelled.Error(), true
		}
		buf, err := json.Marshal(snap)
		if err != nil {
			return err.Error(), true
		}
		return string(buf), false
	}}
}
