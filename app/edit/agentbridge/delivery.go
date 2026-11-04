package agentbridge

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"aego/app/agent"
	"aego/app/agent/mcp"
	gameexport "aego/app/export"
	"aego/editor/core/context"
	"aego/editor/core/services"
	"aego/engine/i18n"
)

func (b *Bridge) deliveryTools() []mcp.Tool {
	return []mcp.Tool{
		{Name: "project_create_empty", Description: "Create a new empty, script-only project beside this project with explicit user approval. The new project has its own editor session.", Schema: visualSchema([]string{"name"}, map[string]any{"name": map[string]any{"type": "string"}}), Output: b.newProject},
		{Name: "project_save_documents", Description: "Ask the editor to save the open documents through their normal save handlers before strict export.", Schema: visualSchema(nil, map[string]any{}), Output: b.saveDocuments},
		{Name: "project_export_game", Description: "Build a release export of this saved, script-only project into a fresh project-local folder. Requires approval independently of Auto-apply.", Schema: visualSchema([]string{"target"}, map[string]any{"target": map[string]any{"type": "string", "enum": []string{"web", "host", "linux", "windows", "darwin"}}}), Output: b.exportGame},
		{Name: "project_run_export", Description: "Launch the last successful export from this conversation on this machine with explicit approval. Web is served on loopback.", Schema: visualSchema(nil, map[string]any{}), Output: b.runExport},
	}
}
func okValue(v any) *agent.Output { return &agent.Output{OK: true, Value: v} }

func (b *Bridge) permit(action string, gen uint64) error {
	return b.permitScoped(action, gen, "")
}

func (b *Bridge) permitScoped(action string, gen uint64, scope string) error {
	if b.cancelled(gen) {
		return errCancelled
	}
	if b.PlanOnly() {
		return errPlanOnly
	}
	if scope != "" {
		if v, ok := b.grants.Load(scope); ok && v == gen {
			return nil
		}
	}
	ch := make(chan int, 1)
	if err := b.main(func() {
		if b.cancelled(gen) || b.PlanOnly() {
			ch <- 1
			return
		}
		buttons := []string{i18n.Tr("editor.agent.allow_once", "Allow once"), i18n.Tr("editor.agent.deny", "Deny")}
		if scope != "" {
			buttons = append(buttons, i18n.Tr("editor.agent.allow_session_scope", "Allow this action for this project conversation"))
		}
		b.ctx.Ask(context.Question{Title: i18n.Tr("editor.agent.project_operation", "Agent project operation"), Message: action + "\n" + b.ctx.Project.Root, Buttons: buttons, Then: func(n int) {
			select {
			case ch <- n:
			default:
			}
		}})
	}); err != nil {
		return err
	}
	timeout := time.NewTimer(b.timeout())
	defer timeout.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case choice := <-ch:
			if choice != 0 && (choice != 2 || scope == "") {
				return errors.New("user declined project operation")
			}
			if b.cancelled(gen) || b.PlanOnly() {
				return errCancelled
			}
			if choice == 2 {
				b.grants.Store(scope, gen)
			}
			return nil
		case <-tick.C:
			if b.cancelled(gen) || b.PlanOnly() {
				return errCancelled
			}
		case <-timeout.C:
			return errExpired
		}
	}
}

var slug = regexp.MustCompile("^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$")

func (b *Bridge) newProject(a map[string]any) (*agent.Output, error) {
	s, _ := a["name"].(string)
	if !slug.MatchString(s) {
		return nil, errors.New("use a project name with 1–64 letters, digits, underscores or hyphens")
	}
	if b.cfg.NewEmptyProject == nil {
		return nil, errors.New("project creator unavailable")
	}
	gen := b.panelGeneration.Load()
	dir := filepath.Join(filepath.Dir(b.Project().Root), s)
	if err := b.permit(i18n.Tr("editor.agent.create_empty_project", "Create and open an empty project at {dir}?", i18n.Str("dir", dir)), gen); err != nil {
		return nil, err
	}
	if b.cancelled(gen) {
		return nil, errCancelled
	}
	if err := os.Mkdir(dir, 0755); err != nil {
		return nil, err
	}
	root, err := b.cfg.NewEmptyProject(dir)
	if err != nil {
		return nil, err
	}
	if b.cfg.SwitchProject != nil {
		if err = b.main(func() {
			if !b.cancelled(gen) {
				b.cfg.SwitchProject(root)
			}
		}); err != nil {
			return nil, err
		}
	}
	return okValue(map[string]string{"project": root, "session": "open the new project to start a separately bound conversation"}), nil
}
func (b *Bridge) saveDocuments(_ map[string]any) (*agent.Output, error) {
	gen := b.panelGeneration.Load()
	if err := b.permit(i18n.Tr("editor.agent.save_documents", "Save all open project documents?"), gen); err != nil {
		return nil, err
	}
	var e error
	if err := b.main(func() {
		if b.cancelled(gen) {
			e = errCancelled
			return
		}
		e = b.ctx.Docs.SaveAll()
	}); err != nil {
		return nil, err
	}
	if e != nil {
		return nil, e
	}
	return okValue("saved"), nil
}

func (b *Bridge) exportGame(a map[string]any) (*agent.Output, error) {
	gen := b.panelGeneration.Load()
	x := b.cfg.Exporter
	target, _ := a["target"].(string)
	if x == nil {
		return nil, errors.New("export service unavailable")
	}
	if !slices.Contains(x.Targets(), target) {
		return nil, errors.New("unsupported export target")
	}
	if b.Project().Native {
		return nil, errors.New("projects with native code require the editor's manual export flow")
	}
	if err := b.permitScoped(i18n.Tr("editor.agent.build_export", "Build a {target} release export of this project?", i18n.Str("target", target)), gen, "export:"+target); err != nil {
		return nil, err
	}
	var snap any
	var e error
	if err := b.main(func() {
		if b.cancelled(gen) {
			e = errCancelled
			return
		}
		if b.ctx.Docs.Dirty() {
			e = errors.New("save open documents before exporting")
			return
		}
		snap = x.Prepare()
	}); err != nil {
		return nil, err
	}
	if e != nil {
		return nil, e
	}
	root, err := filepath.EvalSymlinks(b.Project().Root)
	if err != nil {
		return nil, err
	}
	parent := filepath.Join(root, "exports")
	if err = os.Mkdir(parent, 0755); err != nil && !os.IsExist(err) {
		return nil, err
	}
	real, err := filepath.EvalSymlinks(parent)
	if err != nil || real != parent {
		return nil, errors.New("exports must be a project-local directory, not a symlink")
	}
	dir, err := os.MkdirTemp(parent, "agent-"+target+"-")
	if err != nil {
		return nil, err
	}
	rep, err := x.Export(snap, services.ExportRequest{Target: target, Profile: "release", Out: dir}, func(float32, string) {})
	if err != nil {
		return nil, err
	}
	if rep.Blocked {
		return nil, fmt.Errorf("strict export blocked: %v", rep.Issues)
	}
	if filepath.Clean(rep.Out) != dir {
		return nil, errors.New("export returned an unexpected destination")
	}
	seal, err := gameexport.Seal(dir)
	if err != nil {
		return nil, err
	}
	if err = b.main(func() {
		if !b.cancelled(gen) {
			b.exportSeal = seal
			b.exportPath = dir
			b.exportTarget = target
			b.exportGeneration = gen
		}
	}); err != nil {
		return nil, err
	}
	if b.cancelled(gen) {
		return nil, errCancelled
	}
	return okValue(rep), nil
}
func (b *Bridge) runExport(_ map[string]any) (*agent.Output, error) {
	gen := b.panelGeneration.Load()
	var out, target string
	var seal [32]byte
	if err := b.main(func() {
		if b.exportGeneration == gen {
			out, target, seal = b.exportPath, b.exportTarget, b.exportSeal
		}
	}); err != nil {
		return nil, err
	}
	launch, ok := b.cfg.Exporter.(interface {
		RunVerified(string, string, [32]byte, func() bool) error
	})
	if out == "" || !ok {
		return nil, errors.New("no runnable export from this conversation")
	}
	if err := b.permitScoped(i18n.Tr("editor.agent.launch_export", "Launch the exported game at {path}?", i18n.Str("path", out)), gen, "launch:"+target); err != nil {
		return nil, err
	}
	if b.cancelled(gen) {
		return nil, errCancelled
	}
	if real, e := filepath.EvalSymlinks(out); e != nil || real != out {
		return nil, errors.New("export folder changed; export again")
	}
	if err := launch.RunVerified(out, target, seal, func() bool { return !b.cancelled(gen) }); err != nil {
		return nil, err
	}
	return okValue(map[string]string{"launched": out, "target": strings.TrimSpace(target)}), nil
}
