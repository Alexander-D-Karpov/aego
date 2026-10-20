package agent

import (
	"strconv"

	"aego/asset"
	"aego/project/refs"
	"aego/scene/mutate"
)

func projectCommands() []*Command {
	return []*Command{
		{Name: "project.validate", Doc: "missing assets and nodes, import errors, prefabs and scenes that do not load", Run: (*Session).projectValidate},
	}
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
