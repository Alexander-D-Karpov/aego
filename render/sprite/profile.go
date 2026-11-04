package sprite

import (
	"math/bits"
	"slices"

	"aego/render/shader"
)

// Draws and Uploads count shared operations touching this producer. Bytes counts only its instances.
type SubmissionStats struct{ Draws, Uploads, Bytes uint64 }

type profileStreams struct {
	sink *SubmissionStats
	mask uint8
}

func (b *Batcher) profileAt(sorted int) *SubmissionStats {
	src := int(SortKey(b.sortedKeys[sorted]).Submission())
	if src >= len(b.profile) {
		return nil
	}
	return b.profile[src]
}

func (b *Batcher) ProfileUploads(batches []Batch, lanes, trim, aux bool) {
	if len(b.profile) == 0 {
		return
	}
	for i := range batches {
		run := &batches[i]
		mask := uint8(1)
		size := uint64(InstanceStride)
		if lanes && run.LaneFirst >= 0 {
			mask |= 2
			size += shader.LaneStride
		}
		if trim && run.TrimFirst >= 0 {
			mask |= 4
			size += shader.TrimStride
		}
		if aux && run.AuxFirst >= 0 {
			mask |= 8
			size += AuxStride
		}
		for k := run.First; k < run.First+run.Count; k++ {
			dst := b.profileAt(k)
			if dst == nil {
				continue
			}
			dst.Bytes += size
			found := false
			for i := range b.streams {
				if b.streams[i].sink == dst {
					b.streams[i].mask |= mask
					found = true
					break
				}
			}
			if !found {
				b.streams = append(b.streams, profileStreams{dst, mask})
			}
		}
	}
	for _, s := range b.streams {
		s.sink.Uploads += uint64(bits.OnesCount8(s.mask))
	}
}

func (b *Batcher) ProfileDraw(from, to int) {
	if len(b.profile) == 0 {
		return
	}
	clear(b.drawn)
	b.drawn = b.drawn[:0]
	for i := from; i < to; i++ {
		dst := b.profileAt(i)
		if dst != nil && !slices.Contains(b.drawn, dst) {
			dst.Draws++
			b.drawn = append(b.drawn, dst)
		}
	}
}
