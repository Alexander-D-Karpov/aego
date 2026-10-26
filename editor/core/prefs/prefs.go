// Package prefs holds the editor preferences of the user, shared by every project and the launcher (D345).
package prefs

import (
	"os"
	"path/filepath"
	"time"

	"aego/core/errs"
	"aego/editor/core/agentui"
	"aego/editor/core/document"
	"aego/engine/i18n"
	"aego/format/aetext"
	"aego/ui/theme"
	"aego/update"
)

const (
	File    = "prefs.aetext"
	version = 1
)

type Prefs struct {
	Theme theme.Mode
	// Language is auto, en or ru; auto follows the OS (D348).
	Language string
	// AgentFollow is the Follow button of the Agent panel; AgentResume is how many idle seconds resume following, 0 never (11b5b).
	AgentFollow bool
	AgentResume float64
	// AgentBudgets cap what the agent's read tools return.
	AgentBudgets agentui.Budgets
	// ProxyConfirm makes changes from the CLI and outside agents wait in the Agent panel (editor.proxy.confirm).
	ProxyConfirm bool
	// Updates is how the editor updates itself: mode, channel, skipped versions and the last check (11i4).
	Updates update.Policy
}

func Default() *Prefs {
	return &Prefs{Theme: theme.ModeSystem, Language: i18n.Auto, AgentResume: 30, AgentBudgets: agentui.DefaultBudgets,
		Updates: update.Policy{Mode: update.Auto, Channel: update.Stable}}
}

func Load(dir string) (*Prefs, error) {
	p := Default()
	if dir == "" {
		return p, nil
	}
	data, err := os.ReadFile(filepath.Join(dir, File))
	if os.IsNotExist(err) {
		return p, nil
	}
	if err != nil {
		return p, errs.Wrap(err, errs.EditorDocument, "cannot read the editor preferences")
	}
	doc, err := aetext.Parse(data, File)
	if err != nil || doc.Head != "prefs" {
		return p, err
	}
	if v, ok := doc.Field("theme"); ok {
		name := v.Str
		if name == "" {
			name = v.Ident
		}
		p.Theme = theme.ParseMode(name)
	}
	if v, ok := doc.Field("language"); ok {
		name := v.Str
		if name == "" {
			name = v.Ident
		}
		if name != "" {
			p.Language = name
		}
	}
	if v, ok := doc.Field("agentFollow"); ok {
		p.AgentFollow = v.Bool
	}
	if v, ok := doc.Field("proxyConfirm"); ok {
		p.ProxyConfirm = v.Bool
	}
	if u := doc.Block("updates", ""); u != nil {
		p.Updates = readUpdates(u)
	}
	for _, f := range []struct {
		name string
		dst  *int
	}{{"agentTreeNodes", &p.AgentBudgets.TreeNodes}, {"agentTreeDepth", &p.AgentBudgets.TreeDepth}, {"agentDocResults", &p.AgentBudgets.DocResults}, {"agentScriptLines", &p.AgentBudgets.ScriptLines}} {
		if v, ok := doc.Field(f.name); ok && v.Kind == aetext.VInt {
			*f.dst = max(0, int(v.Int))
		}
	}
	if v, ok := doc.Field("agentResume"); ok {
		switch v.Kind {
		case aetext.VFloat:
			p.AgentResume = max(0, v.Float)
		case aetext.VInt:
			p.AgentResume = max(0, float64(v.Int))
		}
	}
	return p, nil
}

func Save(dir string, p *Prefs) error {
	if dir == "" {
		return nil
	}
	root := &aetext.Node{Head: "prefs", Attrs: []aetext.Field{{Name: "version", Value: aetext.Int(version)}}}
	root.Fields = append(root.Fields, aetext.Field{Name: "theme", Value: aetext.Ident(p.Theme.String())})
	lang := p.Language
	if lang == "" {
		lang = i18n.Auto
	}
	root.Fields = append(root.Fields, aetext.Field{Name: "language", Value: aetext.Ident(lang)})
	root.Fields = append(root.Fields, aetext.Field{Name: "agentFollow", Value: aetext.Bool(p.AgentFollow)})
	root.Fields = append(root.Fields, aetext.Field{Name: "proxyConfirm", Value: aetext.Bool(p.ProxyConfirm)})
	root.Blocks = append(root.Blocks, writeUpdates(p.Updates))
	root.Fields = append(root.Fields, aetext.Field{Name: "agentResume", Value: aetext.Float64(p.AgentResume)})
	b := p.AgentBudgets
	root.Fields = append(root.Fields,
		aetext.Field{Name: "agentTreeNodes", Value: aetext.Int(int64(b.TreeNodes))},
		aetext.Field{Name: "agentTreeDepth", Value: aetext.Int(int64(b.TreeDepth))},
		aetext.Field{Name: "agentDocResults", Value: aetext.Int(int64(b.DocResults))},
		aetext.Field{Name: "agentScriptLines", Value: aetext.Int(int64(b.ScriptLines))})
	return document.WriteAtomic(filepath.Join(dir, File), aetext.Write(root))
}

func readUpdates(n *aetext.Node) update.Policy {
	pol := update.Policy{Mode: update.Auto, Channel: update.Stable}
	if v, ok := n.Field("mode"); ok {
		pol.Mode = update.ParseMode(v.Ident + v.Str)
	}
	if v, ok := n.Field("channel"); ok {
		pol.Channel = update.ParseChannel(v.Ident + v.Str)
	}
	if v, ok := n.Field("skipped"); ok {
		for _, e := range v.List {
			if e.Str != "" {
				pol.Skipped = append(pol.Skipped, e.Str)
			}
		}
	}
	if v, ok := n.Field("checked"); ok {
		pol.LastCheck, _ = time.Parse(time.RFC3339, v.Str)
	}
	return pol
}

func writeUpdates(pol update.Policy) *aetext.Node {
	n := &aetext.Node{Head: "updates"}
	pol.Mode, pol.Channel = update.ParseMode(string(pol.Mode)), update.ParseChannel(string(pol.Channel))
	skipped := make([]aetext.Value, 0, len(pol.Skipped))
	for _, s := range pol.Skipped {
		skipped = append(skipped, aetext.String(s))
	}
	n.Fields = append(n.Fields,
		aetext.Field{Name: "mode", Value: aetext.Ident(string(pol.Mode))},
		aetext.Field{Name: "channel", Value: aetext.Ident(string(pol.Channel))},
		aetext.Field{Name: "skipped", Value: aetext.List(skipped...)})
	if !pol.LastCheck.IsZero() {
		n.Fields = append(n.Fields, aetext.Field{Name: "checked", Value: aetext.String(pol.LastCheck.UTC().Format(time.RFC3339))})
	}
	return n
}
