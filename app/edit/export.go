package edit

import (
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"

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

var webServers sync.Map

func serveWeb(dir string) (string, error) {
	if u, ok := webServers.Load(dir); ok {
		return u.(string), nil
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	files := http.FileServer(http.Dir(dir))
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
		w.Header().Set("Cache-Control", "no-store")
		files.ServeHTTP(w, r)
	})
	go http.Serve(ln, h)
	u := "http://" + ln.Addr().String() + "/"
	webServers.Store(dir, u)
	return u, nil
}

func (x *exporter) Run(out, target string) error {
	path, err := export.Launchable(x.p, target, out)
	if err != nil {
		return err
	}
	t, _ := export.ParseTarget(target)
	switch {
	case t.Web():
		u, err := serveWeb(path)
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
