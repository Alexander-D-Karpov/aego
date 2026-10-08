package icons

import (
	"encoding/json"
	"sort"
	"strconv"

	"aego/core/errs"
	"aego/core/log"
	"aego/render/texture"
	"aego/vfs"
)

type iconJSON struct {
	ID   string `json:"id"`
	Size int    `json:"size"`
	X    int    `json:"x"`
	Y    int    `json:"y"`
	W    int    `json:"w"`
	H    int    `json:"h"`
}

type atlasJSON struct {
	Scale int        `json:"scale"`
	File  string     `json:"file"`
	Icons []iconJSON `json:"icons"`
}

type fileJSON struct {
	Atlases []atlasJSON `json:"atlases"`
}

type key struct {
	id   string
	size int
}

type Atlas struct {
	Scale   float32
	regions map[key]texture.Region
}

type Set struct {
	OnMissing func(id string)
	atlases   []*Atlas
	missing   map[string]bool
}

func Load(files vfs.FS, up *texture.Uploader, dir string) (*Set, error) {
	meta, err := files.ReadFile(dir + "/icons.json")
	if err != nil {
		return nil, err
	}
	var fj fileJSON
	if err := json.Unmarshal(meta, &fj); err != nil {
		return nil, errs.Wrap(err, errs.VFSIO, "icon metadata is not valid json")
	}
	s := &Set{}
	for _, aj := range fj.Atlases {
		f, err := files.Open(dir + "/" + aj.File)
		if err != nil {
			return nil, err
		}
		data, err := texture.DecodePNG(f, true)
		f.Close()
		if err != nil {
			return nil, err
		}
		tex, err := up.Upload(data, texture.UploadOptions{Label: "icons@" + strconv.Itoa(aj.Scale)})
		if err != nil {
			return nil, err
		}
		whole := texture.Whole(tex, data.Width, data.Height)
		a := &Atlas{Scale: float32(aj.Scale), regions: make(map[key]texture.Region, len(aj.Icons))}
		for _, ic := range aj.Icons {
			a.regions[key{ic.ID, ic.Size}] = whole.Sub(ic.X, ic.Y, ic.W, ic.H)
		}
		s.atlases = append(s.atlases, a)
	}
	if len(s.atlases) == 0 {
		return nil, errs.New(errs.VFSNotFound, "icon atlas is empty", log.F("dir", dir))
	}
	sort.Slice(s.atlases, func(i, j int) bool { return s.atlases[i].Scale < s.atlases[j].Scale })
	return s, nil
}

func (s *Set) pick(scale float32) *Atlas {
	if len(s.atlases) == 0 {
		return nil
	}
	for _, a := range s.atlases {
		if a.Scale >= scale {
			return a
		}
	}
	return s.atlases[len(s.atlases)-1]
}

func (s *Set) Region(id string, px, scale float32) (texture.Region, bool) {
	if s == nil {
		return texture.Region{}, false
	}
	if a := s.pick(scale); a != nil {
		size, other := 16, 24
		if px > 20 {
			size, other = 24, 16
		}
		if reg, ok := a.regions[key{id, size}]; ok {
			return reg, true
		}
		if reg, ok := a.regions[key{id, other}]; ok {
			return reg, true
		}
	}
	s.miss(id)
	return texture.Region{}, false
}

func (s *Set) Has(id string) bool {
	if s == nil || len(s.atlases) == 0 {
		return false
	}
	_, ok := s.atlases[0].regions[key{id, 16}]
	return ok
}

func (s *Set) miss(id string) {
	if s.missing == nil {
		s.missing = map[string]bool{}
	}
	if s.missing[id] {
		return
	}
	s.missing[id] = true
	if s.OnMissing != nil {
		s.OnMissing(id)
	}
}
