//go:build !aego_runtime

package docs

import (
	"slices"
	"sort"
)

const (
	SectionStart     = "start"
	SectionGuides    = "guides"
	SectionReference = "reference"

	GroupStart       = "start"
	GroupScriptAPI   = "ref-script"
	GroupComponents  = "components"
	GroupSettings    = "ref-settings"
	GroupDiagnostics = "diagnostics"
	GroupCLI         = "ref-cli"
	GroupProject     = "project"
)

type Group struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Section string `json:"section"`
	Order   int    `json:"order"`
}

var groups = []Group{
	{GroupStart, "Getting started", SectionStart, 0},
	{"editor", "Editor", SectionGuides, 10},
	{"script", "Aego Script", SectionGuides, 20},
	{"scenes", "Scenes & Prefabs", SectionGuides, 30},
	{"gameplay", "Gameplay", SectionGuides, 40},
	{"rendering", "Rendering", SectionGuides, 60},
	{"extending", "Extending Aego", SectionGuides, 70},
	{"tools", "Tools & Agents", SectionGuides, 80},
	{GroupScriptAPI, "Script API", SectionReference, 100},
	{GroupComponents, "Components", SectionReference, 110},
	{GroupSettings, "Project settings", SectionReference, 120},
	{GroupDiagnostics, "Diagnostics", SectionReference, 130},
	{GroupCLI, "CLI", SectionReference, 140},
	{GroupProject, "Project", SectionReference, 150},
}

func Groups() []Group {
	return slices.Clone(groups)
}

func GroupByID(id string) (Group, bool) {
	for _, g := range groups {
		if g.ID == id {
			return g, true
		}
	}
	return Group{}, false
}

func sectionRank(s string) int {
	switch s {
	case SectionStart:
		return 0
	case SectionGuides:
		return 1
	}
	return 2
}

func (r *Registry) Groups() []Group {
	out := append(Groups(), r.extra...)
	sort.SliceStable(out, func(i, j int) bool {
		if a, b := sectionRank(out[i].Section), sectionRank(out[j].Section); a != b {
			return a < b
		}
		return out[i].Order < out[j].Order
	})
	return out
}

func (r *Registry) group(id string) (Group, bool) {
	if g, ok := GroupByID(id); ok {
		return g, true
	}
	for _, g := range r.extra {
		if g.ID == id {
			return g, true
		}
	}
	return Group{}, false
}

func (r *Registry) declareGroup(page string, g Group) {
	if old, ok := r.group(g.ID); ok {
		if old != g {
			r.problems = append(r.problems, Problem{ID: page, Error: true, Message: "group " + g.ID + " is already declared with another title or order"})
		}
		return
	}
	r.extra = append(r.extra, g)
}

func (r *Registry) guideGroups() []string {
	var out []string
	for _, g := range r.Groups() {
		if g.Section != SectionReference {
			out = append(out, g.ID)
		}
	}
	return out
}
