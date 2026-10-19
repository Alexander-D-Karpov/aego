package edit

import (
	"os/exec"
	"runtime"

	"aego/app/bootstrap"
	"aego/app/export"
	"aego/app/projects"
	"aego/asset/pipeline"
	"aego/editor/core/services"
	"aego/engine/problems"
	"aego/project"
)

type exporter struct {
	p      *project.Project
	pl     *pipeline.Pipeline
	engine string
}

func newExporter(p *project.Project, pl *pipeline.Pipeline) services.Exporter {
	if pl == nil {
		return nil
	}
	root, err := projects.EngineRoot("")
	if err != nil {
		return nil
	}
	return &exporter{p: p, pl: pl, engine: root}
}

func (x *exporter) Targets() []string {
	return export.Targets()
}

func (x *exporter) Out(target, profile string) string {
	t, err := export.ParseTarget(target)
	if err != nil {
		return ""
	}
	prof, err := export.ParseProfile(profile)
	if err != nil {
		return ""
	}
	return export.DefaultOut(x.p, t, prof)
}

func (x *exporter) Prepare() any {
	return export.Take(x.pl)
}

func (x *exporter) Check(prep any, profile string) []problems.Diagnostic {
	snap, ok := prep.(*export.Snapshot)
	if !ok {
		return nil
	}
	prof, err := export.ParseProfile(profile)
	if err != nil {
		return nil
	}
	return export.Check(x.p, snap, prof, bootstrap.NeedsNative(x.p))
}

func (x *exporter) Export(prep any, req services.ExportRequest, progress func(float32, string)) (services.ExportReport, error) {
	snap, _ := prep.(*export.Snapshot)
	rep, err := export.Export(export.Options{
		Project: x.p, Engine: x.engine, Target: req.Target, Profile: req.Profile, Out: req.Out,
		Snapshot: snap, Progress: progress,
	})
	var out services.ExportReport
	if rep != nil {
		out = services.ExportReport{Out: rep.Out, Lines: rep.Lines(), Issues: rep.Diagnostics, Blocked: rep.Blocked}
	}
	return out, err
}

func (x *exporter) Reveal(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("explorer", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}
