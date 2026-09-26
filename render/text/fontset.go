package text

import (
	"strconv"

	"aego/core/errs"
	"aego/core/log"
	"aego/core/mathx"
	"aego/render/texture"
	"aego/vfs"
)

type FontSet struct {
	fonts []*Font
}

func (s *FontSet) Add(f *Font) {
	if f != nil {
		s.fonts = append(s.fonts, f)
	}
}

func (s *FontSet) Len() int {
	return len(s.fonts)
}

func (s *FontSet) Fonts() []*Font {
	return s.fonts
}

func (s *FontSet) Closest(px, scale float32) *Font {
	var best *Font
	bestScore := float32(1e30)
	for _, f := range s.fonts {
		score := mathx.Abs(f.Size-px) * 10
		if f.Scale < scale {
			score += (scale - f.Scale) * 100
		} else {
			score += f.Scale - scale
		}
		if score < bestScore {
			best, bestScore = f, score
		}
	}
	return best
}

func LoadSet(files vfs.FS, up *texture.Uploader, dir, prefix string, sizes, scales []int) (*FontSet, error) {
	set := &FontSet{}
	for _, size := range sizes {
		for _, scale := range scales {
			base := dir + "/" + prefix + "-" + strconv.Itoa(size) + "@" + strconv.Itoa(scale)
			if !files.Exists(base+".json") || !files.Exists(base+".png") {
				continue
			}
			f, err := LoadFont(files, up, base+".png", base+".json")
			if err != nil {
				return nil, err
			}
			set.Add(f)
		}
	}
	if set.Len() == 0 {
		return nil, errs.New(errs.VFSNotFound, "no baked ui fonts found, run make genfont",
			log.F("dir", dir), log.F("prefix", prefix))
	}
	return set, nil
}
