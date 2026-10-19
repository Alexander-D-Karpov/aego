package playground

import (
	"path"
	"sort"
	"strings"

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
