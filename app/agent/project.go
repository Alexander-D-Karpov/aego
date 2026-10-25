package agent

import (
	"errors"
	"strconv"
	"strings"

	"aego/format/aetext"
	"aego/project"

	"aego/asset"
	"aego/project/refs"
	"aego/scene/mutate"
)

func projectCommands() []*Command {
	return []*Command{
		{Name: "project.validate", Doc: "missing assets and nodes, import errors, prefabs and scenes that do not load", Run: (*Session).projectValidate},
		{Name: "project.get", Doc: "module sections of project.aego (physics, …) with their current values",
			Args: []Arg{{Name: "section", Type: "string", Doc: "only this section", Pos: true}}, Run: (*Session).projectGet},
		{Name: "project.set", Doc: "set a field of a module section in project.aego, for example physics gravity [0, -15]", Write: true,
			Args: []Arg{{Name: "section", Type: "string", Doc: "section id from project.get", Required: true, Pos: true},
				{Name: "field", Type: "string", Doc: "field name as project.get shows it", Required: true, Pos: true},
				{Name: "value", Type: "string", Doc: `value in project file syntax: 15, [0, -15], Default, "text"`, Pos: true},
				{Name: "reset", Type: "boolean", Doc: "drop the field so the section default applies"}}, Run: (*Session).projectSet},
	}
}

func (s *Session) projectGet(a Args) (*Output, error) {
	secs, err := s.sections()
	if err != nil {
		return nil, err
	}
	only := a.Str("section")
	vals := map[string]string{}
	var b strings.Builder
	for _, d := range secs.List() {
		if only != "" && d.ID != only {
			continue
		}
		n, err := secs.Encode(d.ID, nil)
		if err != nil {
			return nil, err
		}
		text := string(aetext.Write(n))
		vals[d.ID] = text
		b.WriteString(text)
		if !strings.HasSuffix(text, "\n") {
			b.WriteString("\n")
		}
	}
	if only != "" && len(vals) == 0 {
		return nil, errors.New("no project section " + only)
	}
	return &Output{Value: vals, Text: b.String()}, nil
}

// withField sets, or with drop removes, one field of a section block.
func withField(n *aetext.Node, name string, v aetext.Value, drop bool) {
	out := make([]aetext.Field, 0, len(n.Fields)+1)
	set := false
	for _, f := range n.Fields {
		if f.Name != name {
			out = append(out, f)
			continue
		}
		if !drop && !set {
			out = append(out, aetext.Field{Name: name, Value: v})
			set = true
		}
	}
	if !drop && !set {
		out = append(out, aetext.Field{Name: name, Value: v})
	}
	n.Fields = out
}

func fieldText(n *aetext.Node, name string) string {
	if n == nil {
		return ""
	}
	if v, ok := n.Field(name); ok {
		return mutate.FormatValue(v)
	}
	return ""
}

// projectSet changes one field through the section's codec, so a wrong name or value is refused by the section itself (D342).
func (s *Session) projectSet(a Args) (*Output, error) {
	secs, err := s.sections()
	if err != nil {
		return nil, err
	}
	id, field := a.Str("section"), a.Str("field")
	desc, ok := secs.Desc(id)
	if !ok {
		return nil, errors.New("no project section " + id + "; project.get lists them")
	}
	cur, err := secs.Encode(id, nil)
	if err != nil {
		return nil, err
	}
	n := cur.Clone()
	switch {
	case a.Bool("reset"):
		withField(n, field, aetext.Value{}, true)
	case !a.Has("value"):
		return nil, errors.New("project.set needs a value, or reset")
	default:
		v, err := aetext.ParseValue([]byte(a.Str("value")), field)
		if err != nil {
			return nil, err
		}
		withField(n, field, v, false)
	}
	val, err := secs.Decode(id, n)
	if err != nil {
		return nil, err
	}
	norm, err := secs.Encode(id, val)
	if err != nil {
		return nil, err
	}
	block, err := secs.Node(id, val)
	if err != nil {
		return nil, err
	}
	ch := mutate.Change{Op: "project-set", Node: id, Path: field, Before: fieldText(cur, field), After: fieldText(norm, field)}
	if ch.Before == ch.After {
		return &Output{Notes: []string{id + "." + field + " already has this value"}}, nil
	}
	out := &Output{Changes: []mutate.Change{ch}}
	if a.Bool("dry-run") {
		out.DryRun = true
		return out, nil
	}
	if s.host != nil {
		return out, s.host.ApplyProject(id, block, "Set "+id+"."+field)
	}
	if err := s.writable(); err != nil {
		return nil, err
	}
	s.Project.ReplaceBlock(block, desc.Heads()...)
	if err := s.Project.Save(); err != nil {
		return nil, err
	}
	if err := secs.Set(id, val); err != nil {
		return nil, err
	}
	out.Files = []string{project.FileName}
	return out, nil
}

func (s *Session) projectValidate(Args) (*Output, error) {
	out := &Output{}
	add := func(err bool, node, msg string) {
		out.Problems = append(out.Problems, mutate.Problem{Error: err, Node: node, Message: msg})
	}
	for _, m := range refs.MissingAssets(s.Project, s.Pipeline) {
		add(true, "", "missing asset "+m.Label())
	}
	for _, m := range refs.MissingNodes(s.Pipeline) {
		add(true, m.Scene+" "+m.Path, m.Owner+"."+m.Field+" references a node that is not in the scene")
	}
	if s.scriptErr != nil {
		add(true, "", "script types: "+s.scriptErr.Error())
	}
	for _, e := range s.libErrs {
		add(true, "", e.Error())
	}
	for _, x := range s.Pipeline.Assets() {
		if x.Error != "" {
			add(true, x.Path, x.Error)
			continue
		}
		if x.Kind != asset.ArtScene && x.Kind != asset.ArtPrefab {
			continue
		}
		src, err := s.read(x.Path)
		if err != nil {
			add(true, x.Path, err.Error())
			continue
		}
		doc, err := mutate.Load(src, x.Path, s.env())
		if err != nil {
			add(true, x.Path, err.Error())
			continue
		}
		for _, p := range doc.Problems {
			add(p.Error, x.Path+" "+p.Node, p.Message)
		}
	}
	errs, warns := 0, 0
	for _, p := range out.Problems {
		if p.Error {
			errs++
		} else {
			warns++
		}
	}
	out.Notes = []string{"project: " + strconv.Itoa(errs) + " error(s), " + strconv.Itoa(warns) + " warning(s)"}
	return out, nil
}
