package edit

import (
	"testing"

	gameexport "aego/app/export"
	"aego/asset/pipeline"
	"aego/core/log"
	"aego/engine/problems"
)

func TestExportRejectsUnsavedMetadataWorkingOverlay(t *testing.T) {
	p := newProject(t)
	pl, err := pipeline.Open(p, pipeline.NewImporterRegistry(), log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer pl.Close()
	// The loader may display a private Undo generation while the pipeline retains the saved artifact; export must reject this even for .meta changes.
	for _, path := range []string{"assets/platform_height.png.meta", "assets/effects/Dust.aepreset"} {
		t.Run(path, func(t *testing.T) {
			x := &exporter{p: p, pl: pl, dirtySources: func() []string { return []string{path} }}
			snapshot := x.Prepare().(*gameexport.Snapshot)
			if len(snapshot.DirtySources) != 1 || snapshot.DirtySources[0] != path {
				t.Fatal("working generation was silently omitted from export snapshot")
			}
			for _, d := range x.Check(snapshot, "release") {
				if d.Code == gameexport.CodeDirtySource && d.Severity == problems.Error && d.Subject.File == path {
					return
				}
			}
			t.Fatal("export accepted a different generation from the dirty document")
		})
	}
}
