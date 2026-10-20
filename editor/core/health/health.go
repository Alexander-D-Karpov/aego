package health

import (
	"path"
	"sort"
	"strconv"
	"strings"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/engine/problems"
	"aego/project"
)

type Severity uint8

const (
	Info Severity = iota
	Warning
	Error
)

const (
	ActionMissing  = "missing"
	ActionProblems = "problems"
	ActionReveal   = "reveal"
	DefaultTexture = 4096
	PresetsDir     = "assets/presets"
)

type Item struct {
	Code     string
	Severity Severity
	Title    string
	Paths    []string
	Action   string
}

type Env struct {
	Pipeline *pipeline.Pipeline
	Project  *project.Project
	Problems []problems.Diagnostic
	Missing  int
	Exclude  []string
}

func excluded(rel string, list []string) bool {
	for _, p := range list {
		if rel == p || strings.HasPrefix(rel, strings.TrimSuffix(p, "/")+"/") {
			return true
		}
	}
	return false
}

func Run(env Env) []Item {
	var out []Item
	pl := env.Pipeline
	if pl == nil {
		return nil
	}
	assets := pl.Assets()
	sort.Slice(assets, func(i, j int) bool { return assets[i].Path < assets[j].Path })

	var broken []string
	for _, a := range assets {
		if a.Error != "" {
			broken = append(broken, a.Path)
		}
	}
	if len(broken) > 0 {
		out = append(out, Item{Code: "HLT001", Severity: Error, Title: strconv.Itoa(len(broken)) + " asset(s) failed to import", Paths: broken, Action: ActionReveal})
	}

	owners := map[string][2]int{}
	var order []string
	for _, d := range env.Problems {
		if d.Severity == problems.Info {
			continue
		}
		c, seen := owners[d.Owner]
		if !seen {
			order = append(order, d.Owner)
		}
		c[0]++
		if d.BlocksPlay {
			c[1]++
		}
		owners[d.Owner] = c
	}
	sort.Strings(order)
	for _, o := range order {
		c := owners[o]
		sev := Warning
		if c[1] > 0 {
			sev = Error
		}
		out = append(out, Item{Code: "HLT002", Severity: sev, Title: strconv.Itoa(c[0]) + " problem(s) from " + o, Action: ActionProblems})
	}

	if env.Missing > 0 {
		out = append(out, Item{Code: "HLT003", Severity: Error, Title: strconv.Itoa(env.Missing) + " missing reference(s)", Action: ActionMissing})
	}

	reached := map[ids.AssetID]bool{}
	var queue []ids.AssetID
	push := func(id ids.AssetID) {
		if !id.IsZero() && !reached[id] {
			reached[id] = true
			queue = append(queue, id)
		}
	}
	if env.Project != nil {
		push(env.Project.MainScene)
	}
	referenced := map[ids.AssetID]bool{}
	for _, a := range assets {
		if e, ok := pl.Entry(a.ID); ok {
			for _, d := range e.Deps {
				referenced[d] = true
			}
		}
		if a.Kind == asset.ArtScene || a.Kind == asset.ArtPrefab || a.Kind == asset.ArtScript ||
			strings.HasPrefix(a.Path, PresetsDir+"/") || excluded(a.Path, env.Exclude) {
			push(a.ID)
		}
	}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if e, ok := pl.Entry(id); ok {
			for _, d := range e.Deps {
				push(d)
			}
		}
	}
	var unused, idle []string
	for _, a := range assets {
		if !reached[a.ID] {
			unused = append(unused, a.Path)
		}
		if a.Kind == asset.ArtScene && !referenced[a.ID] && (env.Project == nil || a.ID != env.Project.MainScene) {
			idle = append(idle, a.Path)
		}
	}
	if len(unused) > 0 {
		out = append(out, Item{Code: "HLT004", Severity: Warning, Title: strconv.Itoa(len(unused)) + " unused asset(s)", Paths: unused, Action: ActionReveal})
	}
	if len(idle) > 0 {
		out = append(out, Item{Code: "HLT005", Severity: Info, Title: strconv.Itoa(len(idle)) + " scene(s) nothing loads", Paths: idle, Action: ActionReveal})
	}

	byName := map[string][]string{}
	var keys []string
	for _, a := range assets {
		k := strings.ToLower(a.Path)
		if _, ok := byName[k]; !ok {
			keys = append(keys, k)
		}
		byName[k] = append(byName[k], a.Path)
	}
	var dups []string
	for _, k := range keys {
		if l := byName[k]; len(l) > 1 {
			dups = append(dups, l...)
		}
	}
	if len(dups) > 0 {
		out = append(out, Item{Code: "HLT006", Severity: Warning, Title: "names that differ only in case", Paths: dups, Action: ActionReveal})
	}
	return out
}

func Scan(pl *pipeline.Pipeline, cameras []string, limit int) []Item {
	if pl == nil {
		return nil
	}
	if limit <= 0 {
		limit = DefaultTexture
	}
	files := pl.Artifacts()
	var big, dark []string
	for _, a := range pl.Assets() {
		e, ok := pl.Entry(a.ID)
		if !ok {
			continue
		}
		art, ok := e.Artifact(e.Kind)
		if !ok {
			continue
		}
		switch a.Kind {
		case asset.ArtTexture:
			data, err := files.ReadFile(art.Path)
			if err != nil {
				continue
			}
			if t, err := asset.DecodeTexture(data); err == nil && (t.Width > limit || t.Height > limit) {
				big = append(big, a.Path+"  "+strconv.Itoa(t.Width)+"×"+strconv.Itoa(t.Height))
			}
		case asset.ArtScene:
			if len(cameras) == 0 {
				continue
			}
			data, err := files.ReadFile(art.Path)
			if err != nil {
				continue
			}
			has := false
			for _, c := range cameras {
				if strings.Contains(string(data), `component "`+c+`"`) {
					has = true
					break
				}
			}
			if !has {
				dark = append(dark, a.Path)
			}
		}
	}
	sort.Strings(big)
	sort.Strings(dark)
	var out []Item
	if len(big) > 0 {
		out = append(out, Item{Code: "HLT007", Severity: Warning, Title: strconv.Itoa(len(big)) + " texture(s) larger than " + strconv.Itoa(limit) + " px", Paths: big, Action: ActionReveal})
	}
	if len(dark) > 0 {
		out = append(out, Item{Code: "HLT008", Severity: Info, Title: strconv.Itoa(len(dark)) + " scene(s) without a camera", Paths: dark, Action: ActionReveal})
	}
	return out
}

func PathOf(line string) string {
	if i := strings.Index(line, "  "); i > 0 {
		return line[:i]
	}
	return path.Clean(line)
}
