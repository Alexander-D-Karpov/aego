// Package update lets an exported game or a ui/app application update itself from its own release server, signed like Aego's (D362).
package update

import (
	"errors"
	"strings"
	"sync"

	"aego/engine"
	"aego/engine/module"
	es "aego/engine/settings"
	"aego/format/aetext"
	"aego/schema"
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
				{Name: "auto", Label: "Download and install on restart", Value: schema.Enum(0)},
				{Name: "ask", Label: "Ask before downloading", Value: schema.Enum(1)},
				{Name: "off", Label: "Off", Value: schema.Enum(2)}}},
		{ID: KeyChannel, Kind: schema.KindEnum, Default: schema.Enum(0), Title: "Update channel", Doc: "Stable versions, or builds the publisher makes between them.",
			Category: es.CategoryUpdates, Order: 20, Flags: es.User, Choices: []es.Choice{
				{Name: "stable", Label: "Stable", Value: schema.Enum(0)},
				{Name: "nightly", Label: "Nightly", Value: schema.Enum(1)}}},
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

// backend does the platform's part: the native one checks, downloads and installs, the browser one has nothing to update.
type backend interface {
	init(m *Module, ctx *module.InitContext) error
	check(m *Module) error
	download(m *Module)
	apply(m *Module) error
}

// Module checks the game's channel at start, downloads in Auto mode and installs when the game asks or restarts.
type Module struct {
	mu     sync.Mutex
	b      backend
	sec    Section
	eng    *engine.Engine
	st     *es.Store
	mode   es.Key
	ch     es.Key
	phase  Phase
	newVer string
	notes  string
	err    string
	editor bool
	game   string
	ver    string
}

func New() *Module {
	return &Module{b: newBackend()}
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
	if m.sec.URL == "" || m.editor {
		return nil
	}
	return m.b.init(m, ctx)
}

func (m *Module) Shutdown() {}

// Configured reports whether the project names an update server.
func (m *Module) Configured() bool {
	return m.sec.URL != ""
}

// State is the phase, the new version and its notes, and the last error.
func (m *Module) State() (Phase, string, string, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.phase, m.newVer, m.notes, m.err
}

// Check looks for a newer version in the background; Auto mode downloads it.
func (m *Module) Check() error {
	return m.b.check(m)
}

// Download fetches the new version for this system into the user cache.
func (m *Module) Download() {
	m.b.download(m)
}

// Apply installs the downloaded version and starts it in place of this run.
func (m *Module) Apply() error {
	return m.b.apply(m)
}
