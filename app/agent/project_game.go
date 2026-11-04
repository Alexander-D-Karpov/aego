package agent

import (
	"bytes"
	"encoding/json"
	"errors"

	"aego/asset"
	"aego/core/ids"
	"aego/format/aetext"
	"aego/project"
	"aego/scene/mutate"
)

func (s *Session) mainScene(a Args) (*Output, error) {
	rel, err := s.rel(a.Str("file"))
	if err != nil {
		return nil, err
	}
	id, ok := s.Pipeline.Lookup(rel)
	if !ok {
		return nil, errors.New("scene asset not found: " + rel)
	}
	entry, ok := s.Pipeline.Index().Lookup(id)
	if !ok || entry.Missing || entry.Kind != asset.ArtScene || entry.Error != "" {
		return nil, errors.New("main scene must be a valid imported scene")
	}
	from := s.Project.MainScene
	out := &Output{DryRun: a.Bool("dry-run")}
	if from == id {
		return out, nil
	}
	out.Changes = []mutate.Change{{Op: "project-set", Node: project.FileName, Path: "mainScene", Before: s.label(from), After: rel}}
	if out.DryRun {
		return out, nil
	}
	if s.host != nil {
		h, ok := s.host.(interface {
			SetMainScene(string, ids.AssetID, ids.AssetID) error
		})
		if !ok {
			return nil, errors.New("this editor cannot set the main scene")
		}
		return out, h.SetMainScene(rel, from, id)
	}
	if err := s.writable(); err != nil {
		return nil, err
	}
	return out, project.ChangeMainScene(s.Project, from, id)
}

func configName(v aetext.Value) string {
	if v.Kind == aetext.VIdent {
		return v.Ident
	}
	return v.Str
}
func namedBlock(parent *aetext.Node, head, name string) *aetext.Node {
	for _, b := range parent.Blocks {
		if n, ok := b.Field("name"); b.Head == head && ok && configName(n) == name {
			return b
		}
	}
	b := &aetext.Node{Head: head, Fields: []aetext.Field{{Name: "name", Value: aetext.String(name)}}}
	parent.Blocks = append(parent.Blocks, b)
	return b
}

func (s *Session) inputAction(a Args) (*Output, error) {
	var bindings []string
	if err := json.Unmarshal([]byte(a.Str("bind")), &bindings); err != nil || len(bindings) == 0 || len(bindings) > 64 {
		return nil, errors.New("bind must be an array of 1 to 64 control strings")
	}
	name, kind, ctx := a.Str("name"), a.Str("type"), a.Str("context")
	if ctx == "" {
		ctx = "Gameplay"
	}
	if kind != "button" && kind != "axis1" && kind != "axis2" {
		return nil, errors.New("type must be button, axis1 or axis2")
	}
	secs, err := s.sections()
	if err != nil {
		return nil, err
	}
	desc, ok := secs.Desc("input")
	if !ok {
		return nil, errors.New("input module is not enabled")
	}
	cur, err := secs.Encode("input", nil)
	if err != nil {
		return nil, err
	}
	n := cur.Clone()
	action := namedBlock(n, "action", name)
	v := make([]aetext.Value, 0, len(bindings))
	for _, b := range bindings {
		if b == "" {
			return nil, errors.New("empty binding")
		}
		v = append(v, aetext.String(b))
	}
	withField(action, "type", aetext.Ident(kind), false)
	withField(action, "bind", aetext.List(v...), false)
	c := namedBlock(n, "context", ctx)
	members, _ := c.Field("actions")
	found := false
	for _, v := range members.List {
		if configName(v) == name {
			found = true
			break
		}
	}
	if !found {
		withField(c, "actions", aetext.List(append(members.List, aetext.String(name))...), false)
	}
	stack, _ := n.Field("stack")
	if a.Bool("active") || len(stack.List) == 0 {
		enabled := false
		for _, v := range stack.List {
			if configName(v) == ctx {
				enabled = true
				break
			}
		}
		if !enabled {
			withField(n, "stack", aetext.List(append(stack.List, aetext.String(ctx))...), false)
		}
	}
	value, err := secs.Decode("input", n)
	if err != nil {
		return nil, err
	}
	norm, err := secs.Encode("input", value)
	if err != nil {
		return nil, err
	}
	out := &Output{DryRun: a.Bool("dry-run")}
	if bytes.Equal(aetext.Write(cur), aetext.Write(norm)) {
		return out, nil
	}
	label := "Set input action " + name
	out.Changes = []mutate.Change{{Op: "project-set", Node: "input", Path: name, Before: string(aetext.Write(cur)), After: string(aetext.Write(norm))}}
	if out.DryRun {
		return out, nil
	}
	block, err := secs.Node("input", value)
	if err != nil {
		return nil, err
	}
	if s.host != nil {
		return out, s.host.ApplyProject("input", block, label)
	}
	if err := s.writable(); err != nil {
		return nil, err
	}
	before := append([]*aetext.Node(nil), s.Project.Extra...)
	s.Project.ReplaceBlock(block, desc.Heads()...)
	if err := s.Project.Save(); err != nil {
		s.Project.Extra = before
		return nil, err
	}
	return out, secs.Set("input", value)
}
