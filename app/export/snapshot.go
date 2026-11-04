package export

import (
	"slices"

	"aego/app/projects"
	"aego/app/run"
	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/log"
	"aego/engine/problems"
	"aego/project"
	"aego/vfs"
)

type Snapshot struct {
	idx     *asset.Index
	files   vfs.FS
	assets  []pipeline.AssetInfo
	issues  []pipeline.MetaIssue
	current *pipeline.Pipeline
	capture []problems.Diagnostic
	// DirtySources are authoring source/metadata buffers not represented by this imported
	// snapshot. Export waits for them to be saved and validated.
	DirtySources []string
}

func Take(pl *pipeline.Pipeline) *Snapshot {
	issues, _ := pl.MetaIssues()
	s := &Snapshot{assets: pl.Assets(), issues: slices.Clone(issues), current: pl}
	pl.ReadSnapshot(func(idx *asset.Index, files vfs.FS) {
		s.idx = idx
		s.files, s.capture = freezeArtifacts(idx, files)
	})
	return s
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
