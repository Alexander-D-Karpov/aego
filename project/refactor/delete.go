package refactor

import (
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/project"
	"aego/project/refs"
)

func Users(p *project.Project, src refs.Source, list []ids.AssetID) []refs.Ref {
	gone := make(map[ids.AssetID]bool, len(list))
	for _, id := range list {
		gone[id] = true
	}
	seen := map[string]bool{}
	var out []refs.Ref
	for _, id := range list {
		for _, r := range refs.Referrers(p, src, id) {
			if (r.Project || !gone[r.Asset]) && !seen[r.Path] {
				seen[r.Path] = true
				out = append(out, r)
			}
		}
	}
	refs.Sort(out)
	return out
}

func Delete(pl *pipeline.Pipeline, list []ids.AssetID) (*pipeline.Batch, error) {
	all := &pipeline.Batch{Failed: map[ids.AssetID]error{}}
	for _, id := range list {
		b, err := pl.Delete(id)
		if b != nil {
			all.Imported = append(all.Imported, b.Imported...)
			all.Removed = append(all.Removed, b.Removed...)
			all.Retry = append(all.Retry, b.Retry...)
			for k, v := range b.Failed {
				all.Failed[k] = v
			}
		}
		if err != nil {
			return all, err
		}
	}
	return all, nil
}
