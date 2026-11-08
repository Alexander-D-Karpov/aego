package art

// retainedChangeBytes conservatively charges metadata and pixel storage which
// differs between two immutable states. Imported COW tiles and deleted layers
// must be charged too: counting only draft-owned writes lets repeated paste or
// deletion retain an unbounded amount of pixels through Undo.
func retainedChangeBytes(before, after *state) int64 {
	estimate := func(s *state) int64 {
		n := int64(1024 + len(s.Cels)*128 + len(s.Layers)*192 + len(s.Frames)*64 + len(s.Images)*96 + len(s.Tags)*96 + len(s.Slices)*128 + len(s.Palette)*64)
		for _, v := range s.Layers {
			n += int64(len(v.Name))
		}
		for _, v := range s.Tags {
			n += int64(len(v.Name))
		}
		for _, v := range s.Slices {
			n += int64(len(v.Name))
		}
		for _, v := range s.Palette {
			n += int64(len(v.Name))
		}
		for k, v := range s.Properties {
			n += int64(len(k) + len(v) + 64)
		}
		return n
	}
	tiles := map[*tile]uint8{}
	images := map[*pixels]uint8{}
	for _, v := range before.Images {
		images[v] |= 1
		for _, block := range v.Tiles {
			tiles[block] |= 1
		}
	}
	for _, v := range after.Images {
		images[v] |= 2
		for _, block := range v.Tiles {
			tiles[block] |= 2
		}
	}
	n := estimate(before) + estimate(after)
	for _, mask := range tiles {
		if mask != 3 {
			n += TileSize * TileSize * 4
		}
	}
	for im, mask := range images {
		if mask != 3 {
			n += int64(128 + len(im.Tiles)*64)
		}
	}
	return n
}
