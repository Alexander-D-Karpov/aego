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
	GroupScript      = "script"
	GroupScriptAPI   = "ref-script"
	GroupComponents  = "components"
	GroupSettings    = "ref-settings"
	GroupDiagnostics = "diagnostics"
	GroupCLI         = "ref-cli"
	GroupProject     = "project"
	GroupFormats     = "ref-formats"
)

type Group struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Section string `json:"section"`
	Order   int    `json:"order"`
	Icon    string `json:"icon,omitempty"`
	// Titles are the group's title in other languages (D348).
	Titles map[string]string `json:"titles,omitempty"`
}

var groups = []Group{
	{GroupStart, "Getting started", SectionStart, 0, "play.start", nil},
	{"editor", "Editor", SectionGuides, 10, "panel.inspector", nil},
	{GroupScript, "Aego Script", SectionGuides, 20, "file.script", nil},
	{"scenes", "Scenes & Prefabs", SectionGuides, 30, "file.prefab", nil},
	{"gameplay", "Gameplay", SectionGuides, 40, "cat.game", nil},
	{"rendering", "Rendering", SectionGuides, 60, "cat.rendering", nil},
	{"extending", "Extending Aego", SectionGuides, 70, "cat.scripts", nil},
	{"tools", "Tools & Agents", SectionGuides, 80, "panel.console", nil},
	{GroupScriptAPI, "Script API", SectionReference, 100, "code.block", nil},
	{GroupComponents, "Components", SectionReference, 110, "node.empty", nil},
	{GroupSettings, "Project settings", SectionReference, 120, "panel.settings", nil},
	{GroupDiagnostics, "Diagnostics", SectionReference, 130, "panel.problems", nil},
	{GroupCLI, "CLI", SectionReference, 140, "panel.console", nil},
	{GroupFormats, "File formats", SectionReference, 145, "file.data", nil},
	{GroupProject, "Project", SectionReference, 150, "file.folder", nil},
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
	for i := range out {
		if t := r.groupT[out[i].ID]; t != nil {
			out[i].Titles = t
		}
	}
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
		if old.Title != g.Title || old.Section != g.Section || old.Order != g.Order || old.Icon != g.Icon {
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
