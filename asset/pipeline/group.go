package pipeline

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"

	"aego/asset"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
)

func groupID(group string) ids.AssetID {
	sum := sha256.Sum256([]byte("aego.group/" + group))
	return ids.AssetID{Hi: binary.BigEndian.Uint64(sum[0:8]), Lo: binary.BigEndian.Uint64(sum[8:16])}
}

func groupPrefix(kind asset.ArtifactKind) string {
	if kind == asset.ArtAtlasPage {
		return "atlas"
	}
	return string(kind)
}

func (p *Pipeline) dropFromGroup(group string, id ids.AssetID) {
	members := p.groups[group]
	for i, m := range members {
		if m == id {
			p.groups[group] = append(members[:i:i], members[i+1:]...)
			break
		}
	}
	delete(p.groupResults[group], id)
	p.stale[group] = true
}

func (p *Pipeline) finalizeGroup(group string) ([]asset.Entry, map[ids.AssetID]error, error) {
	members := p.groups[group]
	pageID, ok := p.pages[group]
	if !ok {
		pageID = groupID(group)
		p.pages[group] = pageID
	}
	if len(members) == 0 {
		p.index.Delete(pageID)
		delete(p.groupKey, group)
		delete(p.pages, group)
		return nil, nil, nil
	}
	sort.Slice(members, func(i, j int) bool {
		return p.byID[members[i]].path < p.byID[members[j]].path
	})
	key := p.groupCacheKey(group, members)
	if key == p.groupKey[group] {
		return nil, nil, nil
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
			return nil, nil, errs.New(errs.AssetBadImporter, "importer does not support groups",
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
			Path:     r.path,
			Settings: r.meta.Settings,
			Result:   res,
		})
	}
	if imp == nil || len(list) == 0 {
		return nil, nil, nil
	}

	var old []asset.Artifact
	if prev, ok := p.index.Lookup(pageID); ok {
		old = append(old, prev.Artifacts...)
	}
	ctx := &GroupContext{Group: group, PageID: pageID, Log: p.log, Previous: func(name string) ([]byte, bool) {
		for _, a := range old {
			if a.Name == name {
				data, err := p.artifacts.ReadFile(a.Path)
				return data, err == nil
			}
		}
		return nil, false
	}}
	perMember, pages, err := imp.Finalize(ctx, list)
	if err != nil {
		return nil, nil, err
	}

	var pageEntry *asset.Entry
	if len(pages) > 0 {
		kind := pages[0].Kind
		prefix := groupPrefix(kind)
		pageEntry = &asset.Entry{ID: pageID, Kind: kind, Name: prefix + "/" + group, Source: prefix + ":" + group}
		for i, page := range pages {
			name := pageID.String()
			switch {
			case page.Name != "":
				name += "-" + strings.ReplaceAll(page.Name, "/", "_")
			case i > 0:
				name += "_" + hex.EncodeToString([]byte{byte(i)})
			}
			path := "cache://artifacts/" + name + artifactExt(page.Kind)
			if err := p.artifacts.WriteFile(path, page.Data); err != nil {
				return nil, nil, err
			}
			sum := sha256.Sum256(page.Data)
			pageEntry.Artifacts = append(pageEntry.Artifacts, asset.Artifact{
				Kind: page.Kind,
				Name: page.Name,
				Path: path,
				Hash: hex.EncodeToString(sum[:8]),
				Size: int64(len(page.Data)),
			})
		}
	}

	failed := map[ids.AssetID]error{}
	var entries []asset.Entry
	for _, m := range list {
		res, ok := perMember[m.ID]
		if !ok {
			continue
		}
		if res.Error != nil {
			failed[m.ID] = res.Error
			continue
		}
		r := p.byID[m.ID]
		entry, err := p.writeArtifacts(r, res)
		if err != nil {
			return nil, nil, err
		}
		if pageEntry != nil {
			entry.Deps = append(entry.Deps, pageID)
		}
		p.index.Put(entry)
		entries = append(entries, entry)
	}
	if pageEntry != nil {
		if len(failed) > 0 {
			pageEntry.Error = strconv.Itoa(len(failed)) + " member(s) failed to import"
		}
		p.index.Put(*pageEntry)
		entries = append([]asset.Entry{*pageEntry}, entries...)
	}
	p.groupKey[group] = key
	return entries, failed, nil
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
