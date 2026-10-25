package agentbridge

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"

	"aego/app/agent"
	"aego/scene/sceneio"
)

// runDir holds the copies of unsaved scenes that scene.run plays, under the project's .aego.
const runDir = ".aego/tmp/agent"

// sceneRun serves scene.run: an unsaved scene runs as the editor has it, the game itself in Config.Run off the main thread.
func (b *Bridge) sceneRun(a agent.Args) (*agent.Output, error) {
	frames, err := a.Int("frames", 120)
	if err != nil {
		return nil, err
	}
	seed, err := a.Int("seed", 1)
	if err != nil {
		return nil, err
	}
	req := agent.RunRequest{Frames: frames, Seed: uint64(seed), Watch: agent.WatchList(a.Str("watch"))}
	var notes []string
	var ferr error
	if e := b.main(func() { req.Scene, notes, ferr = b.runScene(a.Str("file")) }); e != nil {
		return nil, e
	}
	if ferr != nil {
		return nil, ferr
	}
	rep, err := b.cfg.Run(b.ctx.Project, req)
	if err != nil {
		return nil, err
	}
	out := agent.RunOutput(rep)
	out.Notes = append(out.Notes, notes...)
	return out, nil
}

// runScene is the scene file to run and what the run leaves out; it runs on the main thread.
func (b *Bridge) runScene(file string) (string, []string, error) {
	p := b.ctx.Project
	rel := ""
	if file != "" {
		r, err := b.Rel(file)
		if err != nil {
			return "", nil, err
		}
		if !strings.HasSuffix(r, agent.SceneExt) {
			r += agent.SceneExt
		}
		rel = r
	} else if r, ok := b.ctx.Pipeline.PathOf(p.MainScene); ok {
		rel = r
	}
	var notes []string
	for _, d := range b.ctx.Docs.Scripts() {
		if d.Dirty() {
			notes = append(notes, d.Path+" has unsaved changes the run does not have; the user saves it with Ctrl+S")
		}
	}
	if rel == "" {
		return "", notes, nil
	}
	abs := filepath.Join(p.Root, filepath.FromSlash(rel))
	d := b.ctx.Docs.Find(rel)
	if d == nil || !d.Dirty() {
		if _, err := os.Stat(abs); err != nil {
			return "", nil, errors.New("no scene " + rel)
		}
		return abs, notes, nil
	}
	dir := filepath.Join(p.Root, filepath.FromSlash(runDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, err
	}
	tmp := filepath.Join(dir, path.Base(rel))
	if err := os.WriteFile(tmp, sceneio.Save(d.Scene()), 0o644); err != nil {
		return "", nil, err
	}
	return tmp, append(notes, "ran "+rel+" as it is in the editor, with its unsaved changes"), nil
}
