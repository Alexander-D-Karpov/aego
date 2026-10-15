package context

import (
	"aego/engine/module"
	"aego/engine/problems"
	"aego/scene"
	"aego/scene/sceneio"
)

func (e *Editor) SceneChecks() problems.Source {
	return problems.Source{
		Revision: func() uint64 {
			h := uint64(14695981039346656037)
			for _, d := range e.Docs.Docs() {
				h = (h ^ uint64(d.ID)) * 1099511628211
				h = (h ^ d.Scene().Revision()) * 1099511628211
				h = (h ^ d.Scene().StructureRevision()) * 1099511628211
				h = (h ^ d.History.Version()) * 1099511628211
			}
			return h
		},
		Append: func(dst []problems.Diagnostic) []problems.Diagnostic {
			for _, d := range e.Docs.Docs() {
				dst = module.ValidateScene(d.Scene(), dst)
				dst = prefabChecks(d.Scene(), dst)
			}
			return dst
		},
	}
}

func prefabChecks(s *scene.Scene, dst []problems.Diagnostic) []problems.Diagnostic {
	for _, h := range s.PreOrder() {
		if !s.IsAlive(h) {
			continue
		}
		pi, ok := sceneio.InstanceOf(s, h)
		if !ok {
			continue
		}
		subject := problems.Subject{Kind: problems.SubjectNode, Scene: s, Node: s.ID(h)}
		switch {
		case pi.Missing:
			dst = append(dst, problems.Diagnostic{Owner: ProblemScene, Code: "PFB001", Severity: problems.Warning,
				Message: s.Name(h) + ": prefab " + pi.Prefab.Short() + " is missing, the instance is kept as it is in the file",
				Subject: subject})
		case pi.Owner && !pi.Orphans.Empty():
			dst = append(dst, problems.Diagnostic{Owner: ProblemScene, Code: "PFB002", Severity: problems.Info,
				Message: s.Name(h) + " has overrides whose target is gone from the prefab",
				Subject: subject})
		}
	}
	return dst
}

func (e *Editor) pathOf(s *scene.Scene) string {
	if d := e.Docs.PlayDoc(); d != nil && d.Scene() == s {
		return d.Path
	}
	for _, d := range e.Docs.Docs() {
		if d.Scene() == s {
			return d.Path
		}
	}
	return ""
}

func (e *Editor) row(d *problems.Diagnostic) ScriptProblem {
	sub := &d.Subject
	path := sub.File
	if sub.Scene != nil {
		path = e.pathOf(sub.Scene)
	}
	return ScriptProblem{
		Category: d.Owner, Path: path, Line: sub.Line, Col: sub.Col, Node: sub.Node,
		Code: d.Code, Error: d.Severity == problems.Error, Block: d.BlocksPlay, Message: d.Message, Help: d.Help,
	}
}
