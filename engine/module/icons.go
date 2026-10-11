package module

import (
	"encoding/json"
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
			r.report(set.Owner, "icon source with prefix "+set.Prefix+" has no <id>@<size>x<scale>.png files")
		}
	}
	return known
}
