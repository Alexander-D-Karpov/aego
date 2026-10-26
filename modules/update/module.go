// Package update lets an exported game or a ui/app application update itself from its own release server, signed like Aego's (D362).
package update

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"aego/engine"
	"aego/engine/module"
	es "aego/engine/settings"
	"aego/format/aetext"
	"aego/schema"
	upd "aego/update"
)

const (
	ID           = "update"
	ScriptModule = "aego/update"
	// SectionID is the project.aego block, update { url publicKey }; the settings are named after it.
	SectionID  = ID
	KeyMode    = SectionID + ".mode"
	KeyChannel = SectionID + ".channel"
	Version    = 1
)

// Section is project.aego's update { url="https://…/updates" publicKey="…" }: where the game's channels live and the key that signs them.
type Section struct {
	URL       string
	PublicKey string
}

func decode(n *aetext.Node, s *Section) error {
	for _, f := range n.Fields {
		switch f.Name {
		case "url":
			s.URL = f.Value.Str
		case "publicKey":
			s.PublicKey = f.Value.Str
		}
	}
	return nil
}

func encode(s *Section, n *aetext.Node) {
	if s.URL != "" {
		n.Fields = append(n.Fields, aetext.Field{Name: "url", Value: aetext.String(s.URL)})
	}
	if s.PublicKey != "" {
		n.Fields = append(n.Fields, aetext.Field{Name: "publicKey", Value: aetext.String(s.PublicKey)})
	}
}

func validate(s *Section) error {
	if s.URL != "" && s.PublicKey == "" {
		return errors.New("update: a url needs the publicKey that signs its releases; make one with aego release keygen")
	}
	if s.URL != "" && !strings.HasPrefix(s.URL, "https://") && !strings.HasPrefix(s.URL, "http://127.0.0.1") && !strings.HasPrefix(s.URL, "http://localhost") {
		return errors.New("update: the url must be https")
	}
	return nil
}

// Settings are the player's choices a game with an update server gets in its Settings View: the mode and the channel.
func Settings() []es.Desc {
	return []es.Desc{
		{ID: KeyMode, Kind: schema.KindEnum, Default: schema.Enum(0), Title: "Updates", Doc: "How the game updates itself from its publisher.",
			Category: es.CategoryUpdates, Order: 10, Flags: es.User, Choices: []es.Choice{
				{Name: string(upd.Auto), Label: "Download and install on restart", Value: schema.Enum(0)},
				{Name: string(upd.Ask), Label: "Ask before downloading", Value: schema.Enum(1)},
				{Name: string(upd.Off), Label: "Off", Value: schema.Enum(2)}}},
		{ID: KeyChannel, Kind: schema.KindEnum, Default: schema.Enum(0), Title: "Update channel", Doc: "Stable versions, or builds the publisher makes between them.",
			Category: es.CategoryUpdates, Order: 20, Flags: es.User, Choices: []es.Choice{
				{Name: string(upd.Stable), Label: "Stable", Value: schema.Enum(0)},
				{Name: string(upd.Nightly), Label: "Nightly", Value: schema.Enum(1)}}},
	}
}

// Phase is where the game's update is.
type Phase uint8

const (
	Idle Phase = iota
	Checking
	UpToDate
	Available
	Downloading
	Ready
	Failed
)

var phaseNames = [...]string{"idle", "checking", "upToDate", "available", "downloading", "ready", "failed"}

func (p Phase) String() string {
	if int(p) < len(phaseNames) {
		return phaseNames[p]
	}
	return "idle"
}

// Module checks the game's channel at start, downloads in Auto mode and installs when the game asks or restarts.
type Module struct {
	mu     sync.Mutex
	sec    Section
	eng    *engine.Engine
	st     *es.Store
	mode   es.Key
	ch     es.Key
	phase  Phase
	rel    *upd.Release
	file   string
	err    string
	in     upd.Install
	editor bool
	game   string
	ver    string
}

func New() *Module {
	return &Module{}
}

func (*Module) Info() module.Info {
	return module.Info{ID: ID, Version: engine.Version, Builtin: true, DisplayName: "Updates", DocsOrder: 115, DocsIcon: "cat.settings"}
}

func (m *Module) Register(r *module.Registry) error {
	registerDocs(r)
	r.ProjectSection(module.SectionDesc[Section]{ID: SectionID, Title: "Updates", Order: 900, Version: Version,
		Default: func() Section { return Section{} }, Decode: decode, Encode: encode, Validate: validate})
	if r.Bindings() != nil {
		installScript(r.ScriptModule(ScriptModule), m)
	}
	return nil
}

func (m *Module) Init(ctx *module.InitContext) error {
	m.eng, m.st, m.editor, m.game, m.ver = ctx.Engine, ctx.Settings, ctx.Editor, ctx.GameName, ctx.GameVersion
	if s, ok := ctx.Section(SectionID).(*Section); ok && s != nil {
		m.sec = *s
	}
	if m.sec.URL == "" || m.editor || runtime.GOOS == "js" {
		return nil
	}
	descs := Settings()
	var err error
	if m.mode, err = ctx.Setting(descs[0]); err != nil {
		return err
	}
	if m.ch, err = ctx.Setting(descs[1]); err != nil {
		return err
	}
	if exe, err := os.Executable(); err == nil {
		m.in = upd.DetectGame(exe)
	}
	upd.Clean(m.downloads(), m.ver)
	if m.policy() != upd.Off {
		m.Check()
	}
	return nil
}

func (m *Module) Shutdown() {}

func (m *Module) policy() upd.Mode {
	if m.st == nil || m.mode == 0 {
		return upd.Auto
	}
	return upd.ParseMode(m.st.ChoiceName(m.mode))
}

func (m *Module) channel() upd.Channel {
	if m.st == nil || m.ch == 0 {
		return upd.Stable
	}
	return upd.ParseChannel(m.st.ChoiceName(m.ch))
}

// downloads is where the game's updates are fetched to: UserCacheDir/<game>/updates.
func (m *Module) downloads() string {
	dir, _ := os.UserCacheDir()
	return filepath.Join(dir, m.game, "updates")
}

// Configured reports whether the project names an update server.
func (m *Module) Configured() bool {
	return m.sec.URL != ""
}

// State is the phase, the new version and its notes, and the last error.
func (m *Module) State() (Phase, string, string, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, notes := "", ""
	if m.rel != nil {
		v, notes = m.rel.Version, m.rel.Notes
	}
	return m.phase, v, notes, m.err
}

// Check looks for a newer version in the background; Auto mode downloads it.
func (m *Module) Check() {
	m.mu.Lock()
	if !m.Configured() || m.phase == Checking || m.phase == Downloading {
		m.mu.Unlock()
		return
	}
	m.phase, m.err = Checking, ""
	src := &upd.Source{BaseURL: m.sec.URL, Channel: m.channel(), Key: m.sec.PublicKey}
	m.mu.Unlock()
	go func() {
		r, err := src.Check(context.Background())
		m.mu.Lock()
		switch {
		case err != nil:
			m.phase, m.err = Failed, err.Error()
		case !r.Newer(upd.Build{Version: m.ver}):
			m.phase = UpToDate
		default:
			m.phase, m.rel = Available, r
		}
		auto := m.phase == Available && m.policy() == upd.Auto
		m.mu.Unlock()
		if auto {
			m.Download()
		}
	}()
}

// Download fetches the new version for this system into the user cache.
func (m *Module) Download() {
	m.mu.Lock()
	r := m.rel
	if r == nil || m.phase == Downloading {
		m.mu.Unlock()
		return
	}
	m.phase = Downloading
	m.mu.Unlock()
	go func() {
		kind := upd.KindTarball
		if m.in.Kind == upd.KindAppImage || m.in.Kind == upd.KindApp {
			kind = m.in.Kind
		}
		var file string
		a, err := r.Asset(runtime.GOOS, runtime.GOARCH, kind)
		if err == nil {
			file, err = upd.Download(context.Background(), nil, a, filepath.Join(m.downloads(), r.Version), nil)
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		if err != nil {
			m.phase, m.err = Failed, err.Error()
			return
		}
		m.phase, m.file = Ready, file
	}()
}

// Apply installs the downloaded version and starts it in place of this run.
func (m *Module) Apply() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.phase != Ready || m.rel == nil {
		return errors.New("no update is downloaded")
	}
	if _, err := m.in.Apply(m.file, m.rel.Version); err != nil {
		return err
	}
	cmd := exec.Command(m.in.Launch, os.Args[1:]...)
	if err := cmd.Start(); err != nil {
		return err
	}
	if m.eng != nil {
		m.eng.Quit()
	}
	return nil
}
