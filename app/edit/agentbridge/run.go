package agentbridge

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"aego/app/agent"
	"aego/engine/i18n"
	"aego/project"
)

// runDir holds the copies of unsaved scenes that scene.run plays, under the project's .aego.
const runDir = ".aego/tmp/agent"

// sceneRun serves scene.run: an unsaved scene runs as the editor has it, the game itself in Config.Run off the main thread.
func (b *Bridge) sceneRun(a agent.Args) (*agent.Output, error) {
	generation := b.panelGeneration.Load()
	if b.cfg.ConfirmPlay && b.playGrant.Load() != generation {
		action := i18n.Tr("editor.agent.allow_playtests", "Allow isolated playtests of this project's scripts for this conversation?")
		if b.Project().Native {
			action = i18n.Tr("editor.agent.allow_native_playtests", "Allow isolated playtests with this project's native Go modules for this conversation?")
		}
		if err := b.permit(action, generation); err != nil {
			return nil, err
		}
		b.playGrant.Store(generation)
	}
	frames, err := a.Int("frames", 120)
	if err != nil {
		return nil, err
	}
	seed, err := a.Int("seed", 1)
	if err != nil {
		return nil, err
	}
	req, err := agent.RunRequestOf(a, frames, seed)
	if err != nil {
		return nil, err
	}
	var notes []string
	var overlay map[string][]byte
	var rel string
	var temporary bool
	var ferr error
	if e := b.main(func() {
		file := a.Str("file")
		if file == "" {
			if d := b.ctx.Docs.Active(); d != nil && !d.IsPrefab() {
				file = d.Path
			}
		}
		overlay = b.playOverlay()
		if len(overlay) == 0 {
			req.Scene, notes, temporary, ferr = b.runScene(file)
			return
		}
		if file == "" {
			rel, _ = b.ctx.Pipeline.PathOf(b.Project().MainScene)
		} else {
			rel, ferr = b.Rel(file)
			if ferr == nil && !strings.HasSuffix(rel, agent.SceneExt) {
				rel += agent.SceneExt
			}
		}
		notes = append(notes, "ran the editor's unsaved source buffers in an isolated project")
	}); e != nil {
		return nil, e
	}
	if ferr != nil {
		return nil, ferr
	}
	if temporary {
		defer os.Remove(req.Scene)
	}
	target := b.ctx.Project
	tmp := ""
	if overlay != nil {
		tmp, err = os.MkdirTemp("", "aego-play-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(tmp)
		if err = materializePlay(target.Root, tmp, overlay); err != nil {
			return nil, err
		}
		target, err = project.Load(tmp)
		if err != nil {
			return nil, err
		}
		if rel != "" {
			req.Scene = filepath.Join(tmp, filepath.FromSlash(rel))
		}
	}
	rep, err := b.cfg.Run(target, req)
	if err == nil && tmp != "" && len(rep.Shots) > 0 {
		err = keepShots(b.Project().Root, tmp, rep.Shots)
	}
	if err != nil {
		return nil, err
	}
	if len(rep.Shots) > 0 {
		_ = b.main(func() {
			if !b.cancelled(generation) {
				name := rel
				if name == "" {
					name = a.Str("file")
				}
				if name == "" {
					if d := b.ctx.Docs.Active(); d != nil {
						name = d.Path
					}
				}
				b.rememberShots(&rep, generation, name, req)
			}
		})
	}
	out := agent.RunOutput(rep)
	out.Notes = append(out.Notes, notes...)
	return out, nil
}

// runScene is the scene file to run and what the run leaves out; it runs on the main thread.
func (b *Bridge) runScene(file string) (string, []string, bool, error) {
	p := b.ctx.Project
	rel := ""
	if file != "" {
		r, err := b.Rel(file)
		if err != nil {
			return "", nil, false, err
		}
		if !strings.HasSuffix(r, agent.SceneExt) {
			r += agent.SceneExt
		}
		rel = r
	} else if r, ok := b.ctx.Pipeline.PathOf(p.MainScene); ok {
		rel = r
	}
	var notes []string

	if rel == "" {
		return "", notes, false, nil
	}
	abs := filepath.Join(p.Root, filepath.FromSlash(rel))
	d := b.ctx.Docs.Find(rel)
	if d == nil || !d.Dirty() {
		if _, err := os.Stat(abs); err != nil {
			return "", nil, false, errors.New("no scene " + rel)
		}
		return abs, notes, false, nil
	}
	dir := filepath.Join(p.Root, filepath.FromSlash(runDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, false, err
	}
	f, err := os.CreateTemp(dir, "scene-*.aescene")
	if err != nil {
		return "", nil, false, err
	}
	tmp := f.Name()
	_, writeErr := f.Write(b.ctx.Docs.Snapshot(d))
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		os.Remove(tmp)
		return "", nil, false, errors.Join(writeErr, closeErr)
	}
	return tmp, append(notes, "ran "+rel+" as it is in the editor, with its unsaved changes"), true, nil
}
