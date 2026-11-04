package export

import (
	"bytes"
	"os"

	"aego/asset"

	"aego/core/ids"
	"aego/engine/problems"
	"aego/format/aetext"
	"aego/modules/particles"
	"aego/scene"
)

func particlePreset(raw []byte, path string) bool {
	n, err := aetext.Parse(raw, path)
	if err != nil {
		return false
	}
	var containsEmitter func(*aetext.Node) bool
	containsEmitter = func(node *aetext.Node) bool {
		for _, b := range node.Blocks {
			if b.Head == "component" && b.Label == particles.NameSystem {
				return true
			}
			if b.Head == "node" && containsEmitter(b) {
				return true
			}
		}
		return false
	}
	return containsEmitter(n)
}

func (s *Snapshot) exportSkip(e *asset.Entry) bool {
	if e.Kind != asset.ArtPreset {
		return false
	}
	raw, ok := (&resolver{s: s}).blob(e.ID, e.Kind)
	return !ok || !particlePreset(raw, e.Source)
}

func checkPresetSources(s *Snapshot) []problems.Diagnostic {
	if s.current == nil {
		return nil
	}
	var out []problems.Diagnostic
	s.idx.Each(func(e *asset.Entry) {
		if e.Kind != asset.ArtPreset || e.Missing {
			return
		}
		current, err := os.ReadFile(s.current.Abs(e.Source))
		prepared, ok := (&resolver{s: s}).blob(e.ID, e.Kind)
		old := ok && particlePreset(prepared, e.Source)
		if !old && !particlePreset(current, e.Source) {
			return
		}
		if err != nil {
			out = append(out, diag(CodeDirtySource, problems.Error, "cannot read current particle preset: "+err.Error(), e.Source))
			return
		}
		n, err := aetext.Parse(current, e.Source)
		if err != nil || !ok || !bytes.Equal(prepared, aetext.Write(n)) {
			out = append(out, diag(CodeDirtySource, problems.Error, "particle preset differs from the frozen artifact; save and reimport before export", e.Source))
		}
	})
	return out
}

func (r *resolver) checkParticleInputs(sc *scene.Scene, file string, seen map[ids.AssetID]bool) []problems.Diagnostic {
	var out []problems.Diagnostic
	check := func(id ids.AssetID) {
		if id.IsZero() || seen[id] || !resolves(r.s.idx, id) {
			return
		}
		seen[id] = true
		if err := r.verifyCurrentTexture(id); err != nil {
			out = append(out, diag(CodeDirtySource, problems.Error, "particle texture changed since import: "+err.Error(), file))
		}
	}
	scene.Store[particles.ParticleSystem2D](sc).EachAll(func(_ scene.NodeHandle, c *particles.ParticleSystem2D) {
		check(c.Sprite.ID)
		check(c.Mask.ID)
		for i := range c.Frames {
			check(c.Frames[i].ID)
		}
	})
	return out
}
