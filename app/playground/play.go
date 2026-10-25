package playground

import (
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	playmods "aego/app/assembly/playground"
	"aego/script/bytecode"
	"aego/script/diag"
)

type PlayResult struct {
	Diagnostics []Diagnostic `json:"diagnostics"`
	Error       string       `json:"error,omitempty"`
	Started     bool         `json:"started"`
}

func (p *Playground) SetGame(cfg GameConfig) {
	p.gameCfg = cfg
}

func (p *Playground) Game() *Game {
	return p.game
}

func (p *Playground) host() (*Game, error) {
	if p.game == nil {
		g, err := newGame(p.gameCfg, p.types, p.reg, p.man, p.sections)
		if err != nil {
			return nil, err
		}
		p.game = g
	}
	return p.game, nil
}

func ordered(mods map[string]*bytecode.Module) []*bytecode.Module {
	pkgs := make([]string, 0, len(mods))
	for pkg := range mods {
		pkgs = append(pkgs, pkg)
	}
	sort.Strings(pkgs)
	out := make([]*bytecode.Module, len(pkgs))
	for i, pkg := range pkgs {
		out[i] = mods[pkg]
	}
	return out
}

func (p *Playground) Play(example string, files map[string]string) PlayResult {
	ex, ok := p.proj.find(strings.TrimPrefix(example, ScenePrefix))
	if !ok {
		return PlayResult{Error: "there is no scene example " + example}
	}
	all := make(map[string][]byte, len(ex.files)+len(files))
	for rel, src := range ex.files {
		all[path.Join(ScriptsDir, rel)] = []byte(src)
	}
	if len(files) > 0 {
		placed, err := place(files)
		if err != nil {
			return PlayResult{Diagnostics: failed(err)}
		}
		for k, v := range placed {
			all[k] = v
		}
	}
	p.ws.Sync(all)
	gen, ds := p.ws.Build()
	res := PlayResult{Diagnostics: diagnostics(gen.Sources, ds)}
	if len(gen.Failed) > 0 || diag.HasErrors(ds) {
		return res
	}
	g, err := p.host()
	if err != nil {
		res.Error = err.Error()
		return res
	}
	if err := g.Play(ex.name, ex.scene, ordered(gen.Modules)); err != nil {
		res.Error = err.Error()
		return res
	}
	res.Started = true
	return res
}

func (p *Playground) Bench(count int, animate bool) PlayResult {
	g, err := p.host()
	if err != nil {
		return PlayResult{Error: err.Error()}
	}
	g.Bench(count, animate)
	return PlayResult{Started: true}
}

func (p *Playground) Stop() {
	if p.game != nil {
		p.game.Stop()
	}
}

func (p *Playground) Poll() Poll {
	if p.game == nil {
		return Poll{}
	}
	return p.game.Poll()
}

func (p *Playground) LoseContext(lose bool) bool {
	return p.game != nil && p.game.LoseContext(lose)
}

func (p *Playground) PlayScene(name string, src []byte, code string, prefabs map[string]string) PlayResult {
	mods, err := playmods.DecodeTry(code)
	if err != nil {
		return PlayResult{Error: err.Error()}
	}
	g, err := p.host()
	if err != nil {
		return PlayResult{Error: err.Error()}
	}
	if err := playmods.MountTry(g.loader, prefabs); err != nil {
		return PlayResult{Error: err.Error()}
	}
	if err := g.Play(name, src, mods); err != nil {
		return PlayResult{Error: err.Error()}
	}
	return PlayResult{Started: true}
}

// TryCode compiles the script of a try scene into the form DecodeTry reads.
func (p *Playground) TryCode(script string) (string, error) {
	return p.TryFiles(map[string]string{"main.aes": script})
}

// TryFiles compiles several script files, by name, into the form DecodeTry reads.
func (p *Playground) TryFiles(files map[string]string) (string, error) {
	placed, err := place(files)
	if err != nil {
		return "", err
	}
	p.ws.Sync(placed)
	gen, ds := p.ws.Build()
	if len(gen.Failed) > 0 || diag.HasErrors(ds) {
		for _, d := range diagnostics(gen.Sources, ds) {
			if d.Severity == "error" {
				return "", fmt.Errorf("%s:%d:%d: %s", d.File, d.Line, d.Col, d.Message)
			}
		}
		return "", errors.New("the script does not build")
	}
	return playmods.EncodeTry(ordered(gen.Modules)), nil
}
