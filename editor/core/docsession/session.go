//go:build aego_editor

package docsession

import (
	"bytes"
	"fmt"
	"strings"

	"aego/asset/loader"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/play"
	"aego/editor/core/selection"
	"aego/editor/core/shape"
	"aego/engine"
	"aego/engine/module"
	"aego/engine/problems"
	"aego/render"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/schema"
	"aego/ui/core"
	"aego/ui/dnd"
)

const (
	MaxNodes  = 10_000
	ScenePath = "try.aescene"
)

const channel log.Channel = "docs"

type Options struct {
	Engine    *engine.Engine
	Renderer  *render.Renderer
	Loader    *loader.Loader
	Types     *scene.Types
	Sections  *module.Sections
	Scripts   context.ScriptHost
	UI        *core.Context
	Clipboard context.DataClipboard
	Log       *log.Logger
	OnChange  func(text []byte)
}

type State struct {
	Loaded   bool   `json:"loaded"`
	Running  bool   `json:"running"`
	Paused   bool   `json:"paused"`
	Frames   uint64 `json:"frames"`
	Restarts int    `json:"restarts"`
	Nodes    int    `json:"nodes"`
	CanUndo  bool   `json:"canUndo"`
	CanRedo  bool   `json:"canRedo"`
}

type Session struct {
	opt      Options
	ctx      *context.Editor
	edit     *document.Manager
	doc      *document.SceneDocument
	run      *document.SceneDocument
	sel      *selection.Service
	play     *play.Controller
	origin   []byte
	text     []byte
	pick     string
	unsub    func()
	dirty    bool
	want     bool
	restarts int
	scratch  []ids.NodeID
}

func New(opt Options) *Session {
	if opt.Log == nil {
		opt.Log = log.Nop()
	}
	s := &Session{opt: opt, sel: selection.New()}
	s.edit = s.manager()
	s.ctx = &context.Editor{
		Engine:    opt.Engine,
		Renderer:  opt.Renderer,
		Loader:    opt.Loader,
		Types:     opt.Types,
		Docs:      s.manager(),
		Selection: s.sel,
		Commands:  command.New(),
		UI:        opt.UI,
		DnD:       &dnd.State{},
		Clipboard: opt.Clipboard,
		Log:       opt.Log.With(channel),
		Settings:  context.DefaultSettings(),
		Scripts:   opt.Scripts,
		Collapsed: map[string]bool{},
		Shapes:    shape.NewRegistry(),
		Sections:  opt.Sections,
		Problems:  problems.New(),
	}
	s.play = play.New(s.ctx)
	s.ctx.Play = s.play
	return s
}

func (s *Session) manager() *document.Manager {
	opt := document.Options{Types: s.opt.Types, Log: s.opt.Log}
	if s.opt.Loader != nil {
		opt.Resolver = s.opt.Loader
	}
	return document.NewManager(opt)
}

func (s *Session) Editor() *context.Editor {
	return s.ctx
}

func (s *Session) Controller() *play.Controller {
	return s.play
}

func (s *Session) Text() []byte {
	return s.text
}

func (s *Session) parse(data []byte) (*scene.Scene, error) {
	if s.opt.Loader == nil {
		return nil, errs.New(errs.EditorDocument, "the docs session has no asset loader")
	}
	sc, rep, err := sceneio.Load(data, ScenePath, s.opt.Types, s.opt.Loader, sceneio.LoadOptions{KeepIDs: true})
	if err != nil {
		return nil, err
	}
	for _, w := range rep.Warnings {
		s.ctx.Notify(context.LevelWarn, fmt.Sprintf("%s:%d: %s", ScenePath, w.Pos.Line, w.Msg))
	}
	for _, u := range rep.Unknown {
		s.ctx.Notify(context.LevelWarn, "component "+u+" is not available in the docs runtime")
	}
	return sc, nil
}

func (s *Session) Load(data []byte, pick string) error {
	sc, err := s.parse(data)
	if err != nil {
		return err
	}
	s.origin, s.pick, s.restarts = bytes.Clone(data), pick, 0
	s.install(sc)
	s.want = true
	s.start()
	return nil
}

func (s *Session) Reset() error {
	if s.origin == nil {
		return errs.New(errs.EditorDocument, "no scene is loaded")
	}
	sc, err := s.parse(s.origin)
	if err != nil {
		return err
	}
	s.install(sc)
	s.want = true
	s.start()
	return nil
}

func (s *Session) install(sc *scene.Scene) {
	s.play.Stop()
	if s.unsub != nil {
		s.unsub()
	}
	s.edit = s.manager()
	s.doc = s.edit.New(ScenePath, sc)
	s.ctx.Docs = s.manager()
	s.run = s.ctx.Docs.New(ScenePath, sc)
	s.unsub = s.doc.History.Subscribe(func() { s.dirty = true })
	s.dirty = false
	s.text = sceneio.Save(sc)
	s.sel.Set()
	if n, ok := s.find(s.pick); ok {
		s.Select(n)
	}
	s.emit()
}

func (s *Session) component(full string) (*schema.TypeDescriptor, bool) {
	if full == "" {
		return nil, false
	}
	for _, td := range s.opt.Types.Schema().Components() {
		if td.FullName == full {
			return td, true
		}
	}
	return nil, false
}

func (s *Session) find(full string) (ids.NodeID, bool) {
	sc := s.doc.Scene()
	order := sc.PreOrder()
	if td, ok := s.component(full); ok {
		for _, h := range order {
			if _, has := sc.GetByType(h, td.ID); has {
				return sc.ID(h), true
			}
		}
	}
	for _, h := range order {
		if h != sc.Root() {
			return sc.ID(h), true
		}
	}
	return ids.NodeID{}, false
}

func (s *Session) emit() {
	if s.opt.OnChange != nil {
		s.opt.OnChange(s.text)
	}
}

func (s *Session) start() {
	if s.run == nil {
		return
	}
	if err := s.play.Start(s.run); err != nil {
		s.ctx.Notify(context.LevelError, err.Error())
	}
}

func (s *Session) restart() {
	paused := s.play.State() == play.Paused
	s.start()
	if paused {
		s.play.Pause()
	}
	s.restarts++
}

func (s *Session) TogglePause() {
	if s.play.State() == play.Stopped {
		s.want = true
		s.start()
		return
	}
	s.play.Pause()
}

func (s *Session) prune() {
	if s.doc == nil {
		return
	}
	sc := s.doc.Scene()
	s.sel.Prune(s.doc.ID, func(id ids.NodeID) bool {
		h := sc.Handle(id)
		return !h.IsZero() && sc.IsAlive(h)
	})
}

func (s *Session) Undo() error {
	if s.doc == nil {
		return nil
	}
	err := s.doc.History.Undo()
	s.prune()
	return err
}

func (s *Session) Redo() error {
	if s.doc == nil {
		return nil
	}
	err := s.doc.History.Redo()
	s.prune()
	return err
}

func (s *Session) Set(component, field string, v float64) error {
	if s.doc == nil {
		return errs.New(errs.EditorDocument, "no scene is loaded")
	}
	td, ok := s.component(component)
	if !ok {
		return errs.New(errs.SchemaUnknownType, "unknown component "+component)
	}
	var fd *schema.FieldDescriptor
	for i := range td.Fields {
		if strings.EqualFold(td.Fields[i].Name, field) {
			fd = &td.Fields[i]
			break
		}
	}
	if fd == nil {
		return errs.New(errs.SchemaUnknownType, component+" has no field "+field)
	}
	val := schema.Value{Kind: fd.Kind}
	switch fd.Kind {
	case schema.KindFloat32, schema.KindFloat64:
		val.F = v
	case schema.KindInt32, schema.KindInt64, schema.KindBool, schema.KindEnum:
		val.I = int64(v)
	default:
		return errs.New(errs.EditorCommand, field+" is not a number field")
	}
	nodes := append([]ids.NodeID(nil), s.Selected()...)
	if len(nodes) == 0 {
		n, ok := s.find(component)
		if !ok {
			return errs.New(errs.EditorCommand, "no node has "+component)
		}
		nodes = append(nodes, n)
	}
	return s.doc.Props.Set(nil, nodes, td.ID, fd.ID, val)
}

func (s *Session) Tick(dt float64) {
	if s.doc != nil && s.dirty && !s.doc.History.InTransaction() {
		s.dirty = false
		s.text = sceneio.Save(s.doc.Scene())
		s.emit()
		if s.want {
			s.restart()
		}
	}
	if sc := s.play.Scene(); sc != nil && sc.Stats().Alive > MaxNodes {
		s.play.Stop()
		s.want = false
		s.ctx.Notify(context.LevelError, fmt.Sprintf("the scene grew past %d nodes, the game was stopped", MaxNodes))
	}
	s.play.Tick(dt)
}

func (s *Session) State() State {
	st := State{Loaded: s.doc != nil, Frames: s.play.Frames(), Restarts: s.restarts}
	switch s.play.State() {
	case play.Playing:
		st.Running = true
	case play.Paused:
		st.Running, st.Paused = true, true
	}
	if sc := s.play.Scene(); sc != nil {
		st.Nodes = sc.Stats().Alive - 1
	}
	if s.doc != nil {
		st.CanUndo, st.CanRedo = s.doc.History.CanUndo(), s.doc.History.CanRedo()
	}
	return st
}

func (s *Session) Close() {
	s.play.Stop()
	if s.unsub != nil {
		s.unsub()
		s.unsub = nil
	}
}
