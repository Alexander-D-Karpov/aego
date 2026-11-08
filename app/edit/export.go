package edit

import (
	"errors"
	"os"

	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"aego/app/bootstrap"
	"aego/app/export"
	"aego/app/projects"
	"aego/asset/pipeline"
	"aego/editor/core/services"
	"aego/editor/core/sysopen"
	"aego/engine/problems"
	"aego/project"
)

type exporter struct {
	preview      webPreview
	agentPreview webPreview
	p            *project.Project
	pl           *pipeline.Pipeline
	engine       string
	dirtySources func() []string
}

func newExporter(p *project.Project, pl *pipeline.Pipeline, dirtySources func() []string) services.Exporter {
	if pl == nil {
		return nil
	}
	root, err := projects.EngineRoot("")
	if err != nil {
		return nil
	}
	return &exporter{p: p, pl: pl, engine: root, dirtySources: dirtySources}
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
	snapshot := export.Take(x.pl)
	if x.dirtySources != nil {
		for _, rel := range x.dirtySources() {
			switch strings.ToLower(filepath.Ext(rel)) {
			case ".aemat", ".aeshader", ".glsl", ".vert", ".frag", ".meta", ".aepreset", ".aeart":
				snapshot.DirtySources = append(snapshot.DirtySources, rel)
			}
		}
	}
	return snapshot
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

func (x *exporter) CloseAgentPreview() { x.agentPreview.close() }
func (x *exporter) Close()             { x.preview.close(); x.CloseAgentPreview() }

func (x *exporter) RunVerified(out, target string, want [32]byte, valid func() bool) error {
	if _, err := export.Launchable(x.p, target, out); err != nil {
		return err
	}
	dir, err := export.VerifiedCopy(out, want)
	if err != nil {
		return err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	if !valid() {
		cleanup()
		return errors.New("export request expired")
	}
	path, err := export.Launchable(x.p, target, dir)
	if err != nil {
		cleanup()
		return err
	}
	t, _ := export.ParseTarget(target)
	if t.Web() {
		u, err := x.agentPreview.serve(path, cleanup, valid)
		if err != nil {
			cleanup()
			return err
		}
		if err = sysopen.Open(u); err != nil {
			x.agentPreview.close()
		}
		return err
	}
	var cmd *exec.Cmd
	if t.OS == "darwin" {
		cmd = exec.Command("open", "-W", path)
	} else {
		cmd = exec.Command(path)
		cmd.Dir = filepath.Dir(path)
	}
	if err = cmd.Start(); err != nil {
		cleanup()
		return err
	}
	go func() { _ = cmd.Wait(); cleanup() }()
	return nil
}
func (x *exporter) Run(out, target string) error {
	path, err := export.Launchable(x.p, target, out)
	if err != nil {
		return err
	}
	t, _ := export.ParseTarget(target)
	switch {
	case t.Web():
		u, err := x.preview.serve(path, nil, nil)
		if err != nil {
			return err
		}
		return sysopen.Open(u)
	case t.OS == "darwin":
		return exec.Command("open", path).Start()
	}
	cmd := exec.Command(path)
	cmd.Dir = filepath.Dir(path)
	return cmd.Start()
}
