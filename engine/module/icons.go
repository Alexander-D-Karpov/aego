package module

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path"
	"slices"
	"sort"
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
		return "icon " + p.Icon + " is not used by any component, template or editor view"
	case IconBadName:
		return "icon file " + p.Icon + " is not named <id>@<size>x<scale>.png"
	}
	return "icon source with prefix " + p.Icon + " has no <id>@<size>x<scale>.png files"
}

type IconUse struct {
	ID    string
	Owner string
	Size  int
	By    string
}

type IconReport struct {
	Problems []IconProblem
	Uses     []IconUse
	used     map[string]bool
	seen     map[string]bool
	owners   map[string]string
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

func (r *Registry) markIcon(owner, icon, by string) {
	if r.audit != nil {
		r.audit.used[icon] = true
		r.audit.Uses = append(r.audit.Uses, IconUse{ID: icon, Owner: owner, By: by})
	}
}

func (r *Registry) useIcon(known map[string]bool, owner, icon, by string) {
	if icon == "" {
		return
	}
	if r.audit != nil {
		r.audit.used[icon] = true
		r.audit.Uses = append(r.audit.Uses, IconUse{ID: icon, Owner: owner, By: by})
	}
	if !known[icon] {
		r.iconIssue(IconProblem{Kind: IconMissing, Owner: owner, Icon: icon, By: by})
	}
}

func (r *Registry) scanIcons() map[string]bool {
	known := builtinIcons()
	owners := map[string]string{}
	for id := range known {
		owners[id] = "engine"
	}
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
			if seen[full] {
				return nil
			}
			seen[full] = true
			if !strings.HasPrefix(full, set.Owner+".") && full != "cat."+set.Owner {
				r.report(set.Owner, "icon "+full+" must be named "+set.Owner+".<name> or cat."+set.Owner)
				return nil
			}
			if other, dup := owners[full]; dup {
				r.report(set.Owner, "icon "+full+" is already provided by "+other)
				return nil
			}
			owners[full] = set.Owner
			set.IDs = append(set.IDs, full)
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
	if r.audit != nil {
		r.audit.owners = owners
	}
	return known
}

type IconRow struct {
	ID     string   `json:"id"`
	Owner  string   `json:"owner"`
	Size   int      `json:"size"`
	UsedBy []string `json:"usedBy"`
	Status string   `json:"status"`
}

func (r *IconReport) Rows() []IconRow {
	rows := map[string]*IconRow{}
	get := func(id string) *IconRow {
		row := rows[id]
		if row == nil {
			row = &IconRow{ID: id}
			rows[id] = row
		}
		return row
	}
	for id, owner := range r.owners {
		get(id).Owner = owner
	}
	for _, u := range r.Uses {
		row := get(u.ID)
		row.Size = max(row.Size, u.Size)
		if !slices.Contains(row.UsedBy, u.By) {
			row.UsedBy = append(row.UsedBy, u.By)
		}
		if _, known := r.owners[u.ID]; !known && row.Owner == "" {
			row.Owner = u.Owner
		}
	}
	out := make([]IconRow, 0, len(rows))
	for id, row := range rows {
		_, known := r.owners[id]
		switch {
		case !known:
			row.Status = "missing"
		case len(row.UsedBy) == 0:
			row.Status = "unused"
		default:
			row.Status = "ok"
		}
		if row.Size == 0 {
			row.Size = 16
		}
		sort.Strings(row.UsedBy)
		out = append(out, *row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (r *IconReport) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r.Rows())
}

func (r *IconReport) WriteMarkdown(w io.Writer) {
	fmt.Fprintln(w, "# Icons")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Generated by `aego icons list --out docs/icons.md` from the module registrations and the editor icon list; do not edit by hand.")
	fmt.Fprintln(w, "Sources are `<id>.svg` files in `engine/res/icons/src` and `modules/<id>/res/icons/src`; `make genicons` bakes them into the atlas.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| Icon | Owner | Size | Used by | Status |")
	fmt.Fprintln(w, "|---|---|---|---|---|")
	for _, row := range r.Rows() {
		by := strings.ReplaceAll(strings.Join(row.UsedBy, "; "), "|", "/")
		if by == "" {
			by = "—"
		}
		fmt.Fprintf(w, "| `%s` | %s | %d | %s | %s |\n", row.ID, row.Owner, row.Size, by, row.Status)
	}
}
