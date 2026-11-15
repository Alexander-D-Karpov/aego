package welcome

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"aego/core/mathx"
	"aego/editor/core/document"
	"aego/editor/core/services"
	"aego/engine/i18n"
	"aego/ui/widgets"
)

const (
	ID          = "welcome"
	stateFile   = ".aego/user/getting-started"
	ExamplePath = "scripts/hello/spinner.aes"
)

const example = `package hello

import (
	"aego/input"
	"aego/math"
)

// Spinner turns its node and moves it with A/D or the arrow keys.
// Attach it to a node, press Play, then change Speed in the Inspector.
script Spinner {
	// Turn speed in degrees per second.
	@export(min=0, max=720, unit="deg/s")
	var Speed float = 90

	// Move speed in world units per second.
	@export(min=0, max=20, unit="u/s")
	var Move float = 4

	func Update(dt float) {
		node.SetRotation(node.Rotation() + math.Deg2Rad(Speed)*dt)
		p := node.Position()
		p.X += input.Axis("Move") * Move * dt
		node.SetPosition(p)
	}
}
`

type Check struct {
	ID    string
	Title string
	Hint  string
	Done  func() bool
	Show  func()
}

type Panel struct {
	Reference  func()
	NewProject func()
	Empty      func() bool
	Heading    string
	Outro      string
	root       string
	checks     []Check
	done       map[string]bool
	scroll     float32
	content    float32
}

func statePath(root string) string {
	return filepath.Join(root, filepath.FromSlash(stateFile))
}

func FirstRun(root string) bool {
	_, err := os.Stat(statePath(root))
	return os.IsNotExist(err)
}

func Example(scripts string) (bool, error) {
	found := false
	filepath.WalkDir(scripts, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && filepath.Ext(p) == ".aes" && !emptyScript(p) {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	if found {
		return false, nil
	}
	return true, document.WriteAtomic(filepath.Join(scripts, "hello", "spinner.aes"), []byte(example))
}

var emptyScriptRe = regexp.MustCompile(`^package \w+\s+script \w+ \{\s*\}$`)

// emptyScript is a template's starter script, which does not count as the project's own code.
func emptyScript(path string) bool {
	data, err := os.ReadFile(path)
	return err == nil && emptyScriptRe.Match(bytes.TrimSpace(data))
}

func New(root string, checks []Check) *Panel {
	p := &Panel{root: root, checks: checks, done: map[string]bool{}}
	if data, err := os.ReadFile(statePath(root)); err == nil {
		for _, l := range strings.Split(string(data), "\n") {
			if l = strings.TrimSpace(l); l != "" {
				p.done[l] = true
			}
		}
	}
	return p
}

func (p *Panel) ID() string {
	return ID
}

// RefreshLanguage updates text without resetting checklist state or callbacks.
func (p *Panel) RefreshLanguage(entry func(string) (string, string)) {
	if entry == nil {
		return
	}
	p.Heading, _ = entry("welcome")
	_, p.Outro = entry("welcome/done")
	for i := range p.checks {
		p.checks[i].Title, p.checks[i].Hint = entry("welcome/" + p.checks[i].ID)
	}
}

func (p *Panel) Title() string {
	return i18n.Tr("editor.welcome.title", "Getting Started")
}

func (p *Panel) Save() error {
	keys := make([]string, 0, len(p.done))
	for k := range p.done {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return document.WriteAtomic(statePath(p.root), []byte(strings.Join(keys, "\n")+"\n"))
}

func (p *Panel) Done(id string) bool {
	return p.done[id]
}

func (p *Panel) Mark(id string) {
	if p.done[id] {
		return
	}
	p.done[id] = true
	p.Save()
}

func (p *Panel) count() int {
	n := 0
	for _, c := range p.checks {
		if p.done[c.ID] {
			n++
		}
	}
	return n
}

func (p *Panel) Complete() bool {
	return p.count() == len(p.checks)
}

func (p *Panel) Tick() {
	changed := false
	for _, c := range p.checks {
		if !p.done[c.ID] && c.Done != nil && c.Done() {
			p.done[c.ID] = true
			changed = true
		}
	}
	if changed {
		p.Save()
	}
}

func (p *Panel) Draw(f *services.Frame, r mathx.Rect) {
	ui := f.UI
	th := &ui.Theme
	if ui.MouseOver(ui.ID("scroll"), r) {
		if w := ui.ScrollY(th.RowHeight * 3); w != 0 {
			p.scroll -= w
		}
	}
	p.scroll = mathx.Clamp(p.scroll, 0, max(0, p.content-r.H+th.Padding*2))
	cur := ui.Cursor()
	top := cur.Y
	cur.Y -= p.scroll
	heading := p.Heading
	if heading == "" {
		heading = i18n.Tr("editor.welcome.getting_started_with_aego", "Getting started with Aego")
	}
	widgets.Label(ui, heading)
	if p.NewProject != nil && (p.Empty == nil || p.Empty()) {
		if widgets.Button(ui, i18n.Tr("editor.welcome.create_a_project", "Create a project…")) {
			p.NewProject()
		}
	}
	n := p.count()
	widgets.LabelDim(ui, i18n.Tr("editor.welcome.n_of_checks_done", "{n} of {checks} done", i18n.Str("n", strconv.Itoa(n)), i18n.Str("checks", strconv.Itoa(len(p.checks)))))
	bar := cur.Next(6)
	ui.Rect(bar, th.Widget)
	if len(p.checks) > 0 {
		ui.Rect(mathx.Rect{X: bar.X, Y: bar.Y, W: bar.W * float32(n) / float32(len(p.checks)), H: bar.H}, th.Accent)
	}
	cur.Y += 6
	for i, c := range p.checks {
		ui.PushID(c.ID)
		row := cur.Next(th.RowHeight + 4)
		box := mathx.Rect{X: row.X, Y: row.Y + 4, W: row.H - 8, H: row.H - 8}
		done := p.done[c.ID]
		col := th.Text
		if done {
			ui.Rect(box, th.Ok)
			widgets.CheckMark(ui, box, th.Background)
			col = th.TextDim
		} else {
			ui.Rect(box, th.Widget)
			ui.Outline(box, th.Border)
		}
		bw := float32(84)
		ui.PushClip(mathx.Rect{X: row.X, Y: row.Y, W: max(0, row.W-bw-6), H: row.H})
		ui.Text(strconv.Itoa(i+1)+". "+c.Title, mathx.Vec2{X: box.X + box.W + 8, Y: ui.TextY(row)}, col)
		ui.PopClip()
		if c.Show != nil && !done {
			if widgets.ButtonRect(ui, ui.ID("show"), mathx.Rect{X: row.X + row.W - bw, Y: row.Y + 2, W: bw, H: row.H - 4}, i18n.Tr("editor.welcome.show_me", "Show me")) {
				c.Show()
			}
		}
		if c.Hint != "" {
			cur.X += box.W + 8
			cur.W -= box.W + 8
			widgets.LabelWrapped(ui, c.Hint, true)
			cur.X -= box.W + 8
			cur.W += box.W + 8
		}
		cur.Y += 4
		ui.PopID()
	}
	if p.Complete() && p.Outro != "" {
		widgets.LabelWrapped(ui, p.Outro, false)
	}
	if p.Reference != nil && widgets.Button(ui, i18n.Tr("editor.welcome.open_documentation", "Open Documentation")) {
		p.Reference()
	}
	if widgets.Button(ui, i18n.Tr("editor.welcome.reset_checklist", "Reset checklist")) {
		clear(p.done)
		p.Save()
	}
	p.content = cur.Y + p.scroll - top
}
