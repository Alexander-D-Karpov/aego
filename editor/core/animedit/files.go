package animedit

import (
	"os"

	"aego/asset/pipeline"
	"aego/project"
)

type pipelineFiles struct {
	p     *pipeline.Pipeline
	apply func(*pipeline.Batch)
}

// PipelineFiles reads clips from the project and reimports what it writes through apply.
func PipelineFiles(p *pipeline.Pipeline, apply func(*pipeline.Batch)) Files {
	return pipelineFiles{p: p, apply: apply}
}

func (f pipelineFiles) Read(rel string) ([]byte, error) {
	return os.ReadFile(f.p.Abs(rel))
}

func (f pipelineFiles) Write(rel string, data []byte) error {
	if err := project.WriteAtomic(f.p.Abs(rel), data); err != nil {
		return err
	}
	id, ok := f.p.Lookup(rel)
	if !ok {
		return nil
	}
	b, err := f.p.Reimport(id)
	if err != nil {
		return err
	}
	f.apply(b)
	return nil
}
