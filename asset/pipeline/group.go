package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"

	"aego/asset"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
)

func (p *Pipeline) finalizeGroup(group string) ([]asset.Entry, error) {
	members := p.groups[group]
	if len(members) == 0 {
		return nil, nil
	}
	sort.Slice(members, func(i, j int) bool {
		return p.byID[members[i]].path < p.byID[members[j]].path
	})
	key := p.groupCacheKey(group, members)
	if key == p.groupKey[group] {
		return nil, nil
	}

	var imp GroupImporter
	list := make([]GroupMember, 0, len(members))
	for _, id := range members {
		r, ok := p.byID[id]
		if !ok {
			continue
		}
		gi, ok := r.importer.(GroupImporter)
		if !ok {
			return nil, errs.New(errs.AssetBadImporter, "importer does not support groups",
				log.F("importer", r.importer.ID()), log.F("group", group))
		}
		imp = gi
		res := p.groupResults[group][id]
		if res == nil {
			continue
		}
		list = append(list, GroupMember{
			ID:       id,
			Name:     assetName(r.path),
			Settings: r.meta.Settings,
			Result:   res,
		})
	}
	if imp == nil || len(list) == 0 {
		return nil, nil
	}

	pageID, ok := p.pages[group]
	if !ok {
		pageID = ids.NewAssetID()
		p.pages[group] = pageID
	}
	ctx := &GroupContext{Group: group, PageID: pageID, Log: p.log}
	perMember, pages, err := imp.Finalize(ctx, list)
	if err != nil {
		return nil, err
	}

	var entries []asset.Entry
	pageEntry := asset.Entry{
		ID:     pageID,
		Kind:   asset.ArtAtlasPage,
		Name:   "atlas/" + group,
		Source: "atlas:" + group,
	}
	for i, page := range pages {
		name := pageID.String()
		if i > 0 {
			name += "_" + hex.EncodeToString([]byte{byte(i)})
		}
		path := "cache://artifacts/" + name + ".aetex"
		if err := p.artifacts.WriteFile(path, page.Data); err != nil {
			return nil, err
		}
		sum := sha256.Sum256(page.Data)
		pageEntry.Artifacts = append(pageEntry.Artifacts, asset.Artifact{
			Kind: asset.ArtAtlasPage,
			Path: path,
			Hash: hex.EncodeToString(sum[:8]),
			Size: int64(len(page.Data)),
		})
	}
	p.index.Put(pageEntry)
	entries = append(entries, pageEntry)

	for _, m := range list {
		res, ok := perMember[m.ID]
		if !ok {
			continue
		}
		r := p.byID[m.ID]
		entry, err := p.writeArtifacts(r, res)
		if err != nil {
			return nil, err
		}
		entry.Deps = append(entry.Deps, pageID)
		p.index.Put(entry)
		entries = append(entries, entry)
	}
	p.groupKey[group] = key
	return entries, nil
}

func (p *Pipeline) groupCacheKey(group string, members []ids.AssetID) string {
	h := sha256.New()
	h.Write([]byte(group))
	for _, id := range members {
		r, ok := p.byID[id]
		if !ok {
			continue
		}
		h.Write([]byte(id.String()))
		h.Write([]byte(r.key))
	}
	if len(members) > 0 {
		if gi, ok := p.byID[members[0]].importer.(GroupImporter); ok {
			writeUint(h, uint64(gi.GroupVersion()))
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}
