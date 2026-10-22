package shell

import (
	"math"
	"runtime"
	"strings"

	"aego/core/mathx"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/play"
	"aego/editor/core/workspace"
	edocs "aego/editor/panels/docs"
	"aego/editor/panels/help"
	"aego/editor/panels/welcome"
	"aego/engine"
	"aego/engine/docs"
	"aego/scene"
	"aego/scene/scriptcomp"
	"aego/ui/draw"
)

const (
	CommandGettingStarted = "help.getting_started"
	CommandReference      = "help.reference"
	CommandKeymap         = "help.keymap"
	CommandAbout          = "help.about"
	pulseTime             = 2.5
	pulseTarget           = "play"
	hintErrors            = "hint.errors"
)

type pulse struct {
	target string
	until  float64
}

func (a *App) registerHelp() error {
	list := []command.Desc{
		{ID: CommandGettingStarted, Name: "Getting Started", Category: "Help", Order: 100,
			Run: func() error {
				a.ws.Show(welcome.ID)
				return nil
			}},
		{ID: CommandReference, Name: "Documentation", Category: "Help", Order: 110,
			Run: func() error {
				a.ctx.ShowDocs(edocs.Language, "")
				return nil
			}},
		{ID: CommandKeymap, Name: "Keymap", Category: "Help", Order: 120,
			Run: func() error {
				a.ws.Show(help.KeymapID)
				return nil
			}},
		{ID: CommandAbout, Name: "About Aego", Category: "Help", Order: 900,
			Run: func() error {
				a.ctx.Ask(context.Question{
					Title:   "About Aego",
					Message: "Aego engine " + engine.Version + " · " + runtime.Version() + " · " + runtime.GOOS + "/" + runtime.GOARCH,
					Buttons: []string{"Close"},
				})
				return nil
			}},
	}
	for _, d := range list {
		if err := a.ctx.Commands.Register(d); err != nil {
			return err
		}
	}
	a.play.Subscribe(func(st play.State) {
		if st != play.Stopped && !a.wasPlaying && a.ctx.Scripts != nil {
			a.genAtPlay, _ = a.ctx.Scripts.Status()
		}
		a.wasPlaying = st != play.Stopped
	})
	return nil
}

func (a *App) highlight(target string) {
	a.pulse = pulse{target: target, until: a.ui.Time + pulseTime}
}

func (a *App) showPanel(id string) {
	a.ws.Show(id)
	a.highlight(id)
}

func (a *App) drawPulse() {
	if a.pulse.target == "" || a.ui.Time > a.pulse.until {
		return
	}
	r := a.playRect
	if a.pulse.target != pulseTarget {
		pr, ok := a.ws.Tree.PanelRect(a.pulse.target)
		if !ok {
			return
		}
		r = pr
	}
	col := a.ui.Theme.Accent
	col.A = float32(0.5 + 0.5*math.Sin(a.ui.Time*8))
	l := a.ui.List()
	l.PushLayer(draw.LayerTop)
	l.RectOutline(mathx.Rect{X: r.X - 3, Y: r.Y - 3, W: r.W + 6, H: r.H + 6}, col, 2)
	l.PopLayer()
}

func (a *App) attachedAny() bool {
	for _, d := range a.ctx.Docs.Docs() {
		found := false
		scene.Store[scriptcomp.ScriptComponent](d.Scene()).EachAll(func(_ scene.NodeHandle, sc *scriptcomp.ScriptComponent) {
			if len(sc.Attachments) > 0 {
				found = true
			}
		})
		if found {
			return true
		}
	}
	return false
}

func (a *App) helpEntry(id string) (string, string) {
	if a.opt.Help == nil {
		return id, ""
	}
	e, ok := a.opt.Help.Get(id)
	if !ok {
		return id, ""
	}
	return e.Title, docs.Plain(e.Body)
}

func (a *App) welcomeChecks() []welcome.Check {
	base := map[document.ID]int{}
	checks := []welcome.Check{
		{
			ID: "node",
			Done: func() bool {
				d := a.ctx.Docs.Edit()
				if d == nil {
					return false
				}
				n := d.Scene().Stats().Alive
				b, ok := base[d.ID]
				if !ok {
					base[d.ID] = n
					return false
				}
				return n > b
			},
			Show: func() { a.showPanel(workspace.Hierarchy) },
		},
		{
			ID:   "attach",
			Done: a.attachedAny,
			Show: func() {
				if a.ctx.RevealPath != nil {
					a.ctx.RevealPath("scripts/hello")
				}
				a.showPanel(workspace.Inspector)
			},
		},
		{
			ID:   "play",
			Done: func() bool { return a.play.State() != play.Stopped },
			Show: func() { a.highlight(pulseTarget) },
		},
		{
			ID: "tweak",
			Done: func() bool {
				h := a.play.History()
				return a.play.State() != play.Stopped && h != nil && h.CanUndo()
			},
			Show: func() { a.showPanel(workspace.Inspector) },
		},
		{
			ID: "reload",
			Done: func() bool {
				if a.ctx.Scripts == nil || a.play.State() == play.Stopped {
					return false
				}
				g, _ := a.ctx.Scripts.Status()
				return g > a.genAtPlay
			},
			Show: func() {
				if a.ctx.OpenScript != nil {
					a.ctx.OpenScript(welcome.ExamplePath, 18)
				}
			},
		},
		{
			ID:   "error",
			Done: func() bool { return a.problemsPanel.Opened > 0 },
			Show: func() {
				if a.ctx.ShowProblems != nil {
					a.ctx.ShowProblems("")
				}
				a.highlight(workspace.Problems)
			},
		},
	}
	for i := range checks {
		checks[i].Title, checks[i].Hint = a.helpEntry("welcome/" + checks[i].ID)
	}
	return checks
}

func (a *App) firstRun() {
	p := a.opt.Project
	if !welcome.FirstRun(p.Root) {
		return
	}
	if wrote, err := welcome.Example(p.ScriptsDir()); err != nil {
		a.log.Warn("cannot write the example script: " + err.Error())
	} else if wrote {
		a.ctx.Notify(context.LevelInfo, "Created "+welcome.ExamplePath+" to get you started")
	}
	a.welcome.Save()
	a.ws.Show(welcome.ID)
}

func (a *App) contextHint() string {
	if h := a.nativeHint(); h != "" {
		return h
	}
	f := a.ctx.FocusedPanel
	switch {
	case strings.HasPrefix(f, "script:"):
		if _, _, bad := a.ctx.FirstError(""); bad {
			return "F2 next error · Ctrl+Q explain · Ctrl+Click go to"
		}
		return "Ctrl+Space completion · F1 documentation · Ctrl+Click go to"
	case f == workspace.Scene:
		return "Q W E R tools · F frame selection · Ctrl+D duplicate"
	case f == workspace.Hierarchy:
		return "F2 rename · Ctrl+D duplicate · drag to reparent"
	case f == workspace.Inspector:
		return "Ctrl+Click a script field to open its declaration"
	case f == workspace.Assets:
		return "Double-click to open · F2 rename · drop files to import"
	}
	return "Double Shift searches everywhere · Help → Keymap lists every shortcut"
}
