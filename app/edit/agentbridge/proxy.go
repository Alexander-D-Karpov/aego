package agentbridge

import (
	"encoding/json"

	"aego/app/agent"
	"aego/app/agent/mcp"
	"aego/engine"
	"aego/project"
)

// StartProxy serves the agent commands to the CLI and outside agents while the editor is open, on loopback with its own token.
func (b *Bridge) StartProxy() (url, token string, err error) {
	if b.proxy == nil {
		h, err := mcp.ListenHTTP(mcp.Options{Source: proxySource{b}, Docs: b.cfg.Docs, Version: engine.Version})
		if err != nil {
			return "", "", err
		}
		b.proxy = h
	}
	return b.proxy.URL, b.proxy.Token(), nil
}

type proxySource struct {
	b *Bridge
}

func (p proxySource) Run(name string, a agent.Args) (*agent.Output, error) {
	via := a.Str(agent.ViaArg)
	delete(a, agent.ViaArg)
	if via == "" {
		via = "via CLI"
	}
	return p.b.RunVia(via, p.b.cfg.ProxyConfirm == nil || !p.b.cfg.ProxyConfirm(), name, a)
}

func (p proxySource) Paths() ([]string, error)        { return p.b.Paths() }
func (p proxySource) Project() *project.Project       { return p.b.Project() }
func (p proxySource) Rel(path string) (string, error) { return p.b.Rel(path) }

// RunVia serves a call from outside the panel: reads at once, writes applied at once when direct, else as proposals in the panel.
func (b *Bridge) RunVia(via string, direct bool, name string, a agent.Args) (*agent.Output, error) {
	c, ok := agent.Find(name)
	if ok && b.cfg.OnExternal != nil {
		args, _ := json.Marshal(a)
		b.main(func() { b.cfg.OnExternal(c.Tool(), string(args)) })
	}
	if name == "scene.run" && b.cfg.Run != nil {
		return b.sceneRun(a)
	}
	if ok && c.Write && !a.Bool("dry-run") {
		return b.proposeVia(c, a, via, direct, 0)
	}
	var out *agent.Output
	var err error
	if e := b.main(func() { out, err = b.run(name, a) }); e != nil {
		return nil, e
	}
	return out, err
}
