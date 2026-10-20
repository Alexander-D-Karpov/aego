package export

import (
	"slices"

	"aego/app/projects"
	"aego/app/run"
	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/log"
	"aego/project"
	"aego/vfs"
)

type Snapshot struct {
	idx    *asset.Index
	files  vfs.FS
	assets []pipeline.AssetInfo
	issues []pipeline.MetaIssue
}

func Take(pl *pipeline.Pipeline) *Snapshot {
	issues, _ := pl.MetaIssues()
	return &Snapshot{idx: pl.Index().Clone(), files: pl.Artifacts(), assets: pl.Assets(), issues: slices.Clone(issues)}
}

func Fresh(p *project.Project) (*Snapshot, func(), error) {
	ws, err := projects.OpenWorkspace(p, "", false)
	if err != nil {
		return nil, nil, err
	}
	imps, err := run.Importers(ws)
	if err != nil {
		return nil, nil, err
	}
	pl, err := pipeline.Open(p, imps, log.Nop())
	if err != nil {
		return nil, nil, err
	}
	if _, _, err := pl.Scan(); err != nil {
		pl.Close()
		return nil, nil, err
	}
	if _, err := pl.ImportAll(); err != nil {
		pl.Close()
		return nil, nil, err
	}
	return Take(pl), pl.Close, nil
}

func (s *Snapshot) Revision() uint64 {
	return 1
}

func (s *Snapshot) Index() *asset.Index {
	return s.idx
}

func (s *Snapshot) Artifacts() vfs.FS {
	return s.files
}
