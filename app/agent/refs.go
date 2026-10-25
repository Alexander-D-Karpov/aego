package agent

import (
	"errors"
	"path"
	"strings"
	"time"

	"aego/core/ids"
	"aego/project/refactor"
	"aego/project/refs"
	"aego/scene/mutate"
)

type HitView struct {
	Path    string   `json:"path"`
	Project bool     `json:"project,omitempty"`
	Places  []string `json:"places,omitempty"`
	Error   string   `json:"error,omitempty"`
}

type MissingView struct {
	Asset   string   `json:"asset"`
	Was     string   `json:"was,omitempty"`
	UsedBy  []string `json:"usedBy"`
	Suggest string   `json:"suggest,omitempty"`
}

type MissingNodeView struct {
	File   string `json:"file"`
	Node   string `json:"node"`
	Field  string `json:"field"`
	Target string `json:"target"`
}

type AssetView struct {
	ID    string `json:"id"`
	Path  string `json:"path"`
	Kind  string `json:"kind"`
	Error string `json:"error,omitempty"`
}

func refsCommands() []*Command {
	asset := Arg{Name: "asset", Type: "string", Doc: "asset id or path", Required: true, Pos: true}
	return []*Command{
		{Name: "refs.find", Doc: "files and fields that reference an asset", Args: []Arg{asset}, Run: (*Session).refsFind},
		{Name: "refs.missing", Doc: "references to assets and nodes that do not exist", Run: (*Session).refsMissing},
		{Name: "refs.fix", Doc: "point every reference to one asset at another (none clears them); originals go to .aego/recovery", Write: true, Destroys: true,
			Args: []Arg{{Name: "from", Type: "string", Doc: "asset id or path being replaced", Required: true, Pos: true},
				{Name: "to", Type: "string", Doc: "replacement id or path, empty clears", Pos: true}}, Run: (*Session).refsFix},
		{Name: "asset.list", Doc: "assets with their ids and kinds", Args: []Arg{{Name: "kind", Type: "string", Doc: "only this kind: scene, prefab, texture, data, table, script…"}}, Run: (*Session).assetList},
		{Name: "asset.move", Doc: "move an asset with its .meta; the id and every reference stay", Write: true, Destroys: true,
			Args: []Arg{asset, {Name: "to", Type: "string", Doc: "new path", Required: true, Pos: true}}, Run: (*Session).assetMove},
		{Name: "asset.rename", Doc: "rename an asset in its folder", Write: true, Destroys: true,
			Args: []Arg{asset, {Name: "name", Type: "string", Doc: "new name without the extension", Required: true, Pos: true}}, Run: (*Session).assetRename},
		{Name: "asset.delete", Doc: "delete an asset; refuses while it is referenced unless --force", Write: true, Destroys: true,
			Args: []Arg{asset, {Name: "force", Type: "boolean", Doc: "delete even when referenced; the references become missing"}}, Run: (*Session).assetDelete},
	}
}

func (s *Session) refsFind(a Args) (*Output, error) {
	id, err := s.asset(a.Str("asset"))
	if err != nil {
		return nil, err
	}
	var out []HitView
	for _, h := range refs.Find(s.Project, s.Pipeline, id) {
		v := HitView{Path: h.Ref.Path, Project: h.Ref.Project}
		for _, p := range h.Places {
			v.Places = append(v.Places, p.String())
		}
		if h.Err != nil {
			v.Error = h.Err.Error()
		}
		out = append(out, v)
	}
	return &Output{Value: out}, nil
}

func (s *Session) refsMissing(Args) (*Output, error) {
	out := &Output{}
	assets := []MissingView{}
	for _, m := range refs.MissingAssets(s.Project, s.Pipeline) {
		v := MissingView{Asset: m.ID.String()}
		if m.Buried {
			v.Was = m.Grave.Path
		}
		if !m.Suggest.IsZero() {
			v.Suggest = m.Path
		}
		for _, r := range m.Refs {
			v.UsedBy = append(v.UsedBy, r.Path)
		}
		assets = append(assets, v)
		out.Problems = append(out.Problems, mutate.Problem{Error: true, Message: "missing asset " + m.Label() + ", used by " + strings.Join(v.UsedBy, ", ")})
	}
	nodes := []MissingNodeView{}
	for _, m := range refs.MissingNodes(s.Pipeline) {
		nodes = append(nodes, MissingNodeView{File: m.Scene, Node: m.Path, Field: m.Owner + "." + m.Field, Target: "#" + m.Target.String()})
		out.Problems = append(out.Problems, mutate.Problem{Error: true, Node: m.Scene + " " + m.Path, Message: m.Owner + "." + m.Field + " references a node that is not in the scene"})
	}
	out.Value = map[string]any{"assets": assets, "nodes": nodes}
	return out, nil
}

func (s *Session) refsFix(a Args) (*Output, error) {
	from, err := s.asset(a.Str("from"))
	if err != nil {
		return nil, err
	}
	var to ids.AssetID
	if a.Has("to") {
		if to, err = s.asset(a.Str("to")); err != nil {
			return nil, err
		}
	}
	r, err := refactor.PlanReplace(s.Project, s.Pipeline, from, to, nil)
	if err != nil {
		return nil, err
	}
	edits, err := r.Edits(s.Pipeline.Abs)
	if err != nil {
		return nil, err
	}
	out := &Output{}
	if a.Bool("dry-run") {
		out.DryRun = true
		for _, e := range edits {
			out.Notes = append(out.Notes, "would write "+e.Path)
		}
		if r.Project && !to.IsZero() {
			out.Notes = append(out.Notes, "would set mainScene")
		}
		return out, nil
	}
	backup := refactor.BackupDir(time.Now())
	out.Files, err = refactor.WriteEdits(s.Project.Root, s.Pipeline.Abs, backup, edits)
	if err != nil {
		return out, err
	}
	if err := r.ApplyProject(s.Project); err != nil {
		return out, err
	}
	if r.Project && !to.IsZero() {
		out.Notes = append(out.Notes, "mainScene is now "+s.label(to))
	}
	if len(out.Files) > 0 {
		out.Notes = append(out.Notes, "originals in "+backup)
	}
	_, err = refactor.Reimport(s.Pipeline, out.Files)
	s.reload()
	return out, err
}

func (s *Session) assetList(a Args) (*Output, error) {
	kind := a.Str("kind")
	var list []AssetView
	var b strings.Builder
	for _, x := range s.Pipeline.Assets() {
		if kind != "" && string(x.Kind) != kind {
			continue
		}
		list = append(list, AssetView{ID: x.ID.String(), Path: x.Path, Kind: string(x.Kind), Error: x.Error})
		b.WriteString(x.ID.String() + "  " + string(x.Kind) + "  " + x.Path + "\n")
	}
	return &Output{Value: list, Text: b.String()}, nil
}

func (s *Session) move(a Args, id ids.AssetID, to string) (*Output, error) {
	if a.Bool("dry-run") {
		if s.exists(to) {
			return nil, errors.New(to + " already exists")
		}
		return &Output{DryRun: true, Notes: []string{"would move " + s.label(id) + " to " + to}}, nil
	}
	note, err := refactor.Move(s.Project, s.Pipeline, id, to)
	if err != nil {
		return nil, err
	}
	out := &Output{Files: []string{to}}
	if note != "" {
		out.Notes = append(out.Notes, note)
	}
	s.reload()
	return out, nil
}

func (s *Session) assetMove(a Args) (*Output, error) {
	id, err := s.asset(a.Str("asset"))
	if err != nil {
		return nil, err
	}
	to, err := s.rel(a.Str("to"))
	if err != nil {
		return nil, err
	}
	return s.move(a, id, to)
}

func (s *Session) assetRename(a Args) (*Output, error) {
	id, err := s.asset(a.Str("asset"))
	if err != nil {
		return nil, err
	}
	rel, ok := s.Pipeline.PathOf(id)
	if !ok {
		return nil, errors.New("the asset has no file")
	}
	name := a.Str("name")
	if strings.ContainsAny(name, `/\`) {
		return nil, errors.New("a name cannot contain a path separator, use asset move")
	}
	return s.move(a, id, path.Join(path.Dir(rel), name+path.Ext(rel)))
}

func (s *Session) assetDelete(a Args) (*Output, error) {
	id, err := s.asset(a.Str("asset"))
	if err != nil {
		return nil, err
	}
	rel := s.label(id)
	if users := refactor.Users(s.Project, s.Pipeline, []ids.AssetID{id}); len(users) > 0 && !a.Bool("force") {
		var names []string
		for _, u := range users {
			names = append(names, u.Path)
		}
		return nil, errors.New(rel + " is referenced by " + strings.Join(names, ", ") + "; pass --force to delete it anyway")
	}
	if a.Bool("dry-run") {
		return &Output{DryRun: true, Notes: []string{"would delete " + rel}}, nil
	}
	if _, err := refactor.Delete(s.Pipeline, []ids.AssetID{id}); err != nil {
		return nil, err
	}
	s.reload()
	return &Output{Files: []string{rel}, Notes: []string{"deleted " + rel + ", its id stays in the graveyard for refs missing"}}, nil
}
