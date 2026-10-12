package module

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strconv"
	"strings"

	"aego/engine/res"
)

type IconSet struct {
	Owner  string
	FS     fs.FS
	Prefix string
	IDs    []string
}

func iconID(name string) (string, bool) {
	base, ok := strings.CutSuffix(name, ".png")
	if !ok {
		return "", false
	}
	id, variant, ok := strings.Cut(base, "@")
	if !ok || id == "" {
		return "", false
	}
	size, scale, ok := strings.Cut(variant, "x")
	if !ok {
		return "", false
	}
	if _, err := strconv.Atoi(size); err != nil {
		return "", false
	}
	if _, err := strconv.Atoi(scale); err != nil {
		return "", false
	}
	return id, true
}

func builtinIcons() map[string]bool {
	out := map[string]bool{}
	data, err := fs.ReadFile(res.FS(), "icons/icons.json")
	if err != nil {
		return out
	}
	var f struct {
		Atlases []struct {
			Icons []struct {
				ID string `json:"id"`
			} `json:"icons"`
		} `json:"atlases"`
	}
	if json.Unmarshal(data, &f) != nil {
		return out
	}
	for _, a := range f.Atlases {
		for _, ic := range a.Icons {
			out[ic.ID] = true
		}
	}
	return out
}

type IconProblemKind uint8

const (
	IconMissing IconProblemKind = iota
	IconUnused
	IconBadName
	IconEmptySource
)

type IconProblem struct {
	Kind  IconProblemKind
	Owner string
	Icon  string
	By    string
}

func (p IconProblem) String() string {
	switch p.Kind {
	case IconMissing:
		return p.By + " uses icon " + p.Icon + ", which no icon source provides"
	case IconUnused:
		return "icon " + p.Icon + " is not used by any component or template"
	case IconBadName:
		return "icon file " + p.Icon + " is not named <id>@<size>x<scale>.png"
	}
	return "icon source with prefix " + p.Icon + " has no <id>@<size>x<scale>.png files"
}

type IconReport struct {
	Problems []IconProblem
	used     map[string]bool
	seen     map[string]bool
}

func (r *IconReport) Write(w io.Writer) int {
	for _, p := range r.Problems {
		fmt.Fprintln(w, p.Owner+": "+p.String())
	}
	if len(r.Problems) > 0 {
		return 1
	}
	return 0
}

func (r *Registry) iconIssue(p IconProblem) {
	switch {
	case r.audit != nil:
		key := strconv.Itoa(int(p.Kind)) + "|" + p.Owner + "|" + p.Icon
		if !r.audit.seen[key] {
			r.audit.seen[key] = true
			r.audit.Problems = append(r.audit.Problems, p)
		}
	case p.Kind == IconBadName:
		r.Warn(p.Owner, p.String())
	default:
		r.report(p.Owner, p.String())
	}
}

func (r *Registry) useIcon(known map[string]bool, owner, icon, by string) {
	if icon == "" {
		return
	}
	if r.audit != nil {
		r.audit.used[icon] = true
	}
	if !known[icon] {
		r.iconIssue(IconProblem{Kind: IconMissing, Owner: owner, Icon: icon, By: by})
	}
}

func (r *Registry) scanIcons() map[string]bool {
	known := builtinIcons()
	for i := range r.icons {
		set := &r.icons[i]
		seen := map[string]bool{}
		err := fs.WalkDir(set.FS, ".", func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			id, ok := iconID(path.Base(p))
			if !ok {
				if strings.HasSuffix(p, ".png") {
					r.iconIssue(IconProblem{Kind: IconBadName, Owner: set.Owner, Icon: p})
				}
				return nil
			}
			full := set.Prefix + id
			if !seen[full] {
				seen[full] = true
				set.IDs = append(set.IDs, full)
			}
			known[full] = true
			return nil
		})
		if err != nil {
			r.report(set.Owner, "icon source cannot be read: "+err.Error())
			continue
		}
		if len(set.IDs) == 0 {
			r.iconIssue(IconProblem{Kind: IconEmptySource, Owner: set.Owner, Icon: set.Prefix})
		}
	}
	return known
}
