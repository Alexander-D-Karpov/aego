package document

import (
	"encoding/hex"
	"errors"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"aego/asset"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/history"
	"aego/editor/core/props"
	"aego/editor/core/selection"
	"aego/engine"
	"aego/engine/i18n"
	"aego/engine/module"
	"aego/project"
	"aego/scene"
	"aego/scene/sceneio"
)

type ID = selection.DocID

const (
	RecoveryDir = ".aego/recovery"
	SceneExt    = ".aescene"
	PrefabExt   = ".aeprefab"
)

type Camera struct {
	Position mathx.Vec2
	Zoom     float32
	Rotation float32
}

type ViewState struct {
	Camera   Camera
	Expanded map[ids.NodeID]bool
	Mode     ViewMode
	Orbit    Orbit
	Isolated map[ids.NodeID]bool
}

type LoadStats struct {
	Took  time.Duration
	Nodes int
}

type SceneDocument struct {
	Load       LoadStats
	ID         ID
	Path       string
	History    *history.History
	Props      *props.Access
	View       ViewState
	DiskAt     time.Time
	DiskSize   int64
	OnDisk     bool
	scene      *scene.Scene
	forced     bool
	removedAck bool
	play       bool
}

func (d *SceneDocument) Scene() *scene.Scene {
	return d.scene
}

func (d *SceneDocument) Name() string {
	return strings.TrimSuffix(path.Base(d.Path), SceneExt)
}

func (d *SceneDocument) Dirty() bool {
	return d.forced || d.History.Dirty()
}

func (d *SceneDocument) MarkDirty() {
	d.forced = true
}

func (d *SceneDocument) setScene(s *scene.Scene) {
	d.scene = s
	d.Props = props.New(s, d.History)
}

func (d *SceneDocument) PlayCopy(s *scene.Scene) *SceneDocument {
	p := &SceneDocument{
		ID:       d.ID,
		Path:     d.Path,
		View:     d.View,
		DiskAt:   d.DiskAt,
		DiskSize: d.DiskSize,
		OnDisk:   d.OnDisk,
		play:     true,
	}
	p.History = history.New(p)
	p.History.SetMode(history.ModePlay)
	p.setScene(s)
	return p
}

func (d *SceneDocument) IsPlay() bool {
	return d.play
}

type EventKind uint8

const (
	EvOpened EventKind = iota
	EvClosed
	EvSaved
	EvReloaded
	EvActivated
	EvSourceSaved
	EvRenamed
	// EvScriptReplaced means the shared Code pointer has been replaced, not
	// merely edited. Subscribers must discard source-relative cached state.
	EvScriptReplaced
)

type Event struct {
	Kind    EventKind
	Doc     ID
	Path    string
	OldPath string
}

type Conflict struct {
	Doc     ID
	Path    string
	Removed bool
}

type Recovery struct {
	Path string
	File string
	Time time.Time
}

type Options struct {
	Root     string
	Types    *scene.Types
	Resolver asset.Resolver
	Log      *log.Logger
	OnChange func(*module.ChangeSet)
	// SourceWrite serializes the immediate disk check/write with the host's
	// source transaction guard. It must invoke the callback synchronously; the
	// callback itself must not publish events or reenter the guard.
	SourceWrite func(write func() error) error
}

type subscriber struct {
	id uint64
	fn func(Event)
}

type Manager struct {
	opt             Options
	log             *log.ChannelLogger
	docs            []*SceneDocument
	scripts         []*ScriptDocument
	assets          []*AssetDocument
	handlers        HandlerRegistry
	active          ID
	next            ID
	subs            []subscriber
	nextSub         uint64
	playDoc         *SceneDocument
	playWorld       *engine.World
	origin          map[ID]ID
	back            map[ID]ID
	guard           func(*SceneDocument) func()
	recoveryWritten map[string][32]byte
}

func (m *Manager) SetSaveGuard(fn func(*SceneDocument) func()) {
	m.guard = fn
}

func (m *Manager) guarded(d *SceneDocument, enc func(*SceneDocument) []byte) []byte {
	if m.guard != nil {
		if after := m.guard(d); after != nil {
			defer after()
		}
	}
	return enc(d)
}

func recoveryBytes(d *SceneDocument) []byte {
	return sceneio.Save(d.scene)
}

func NewManager(opt Options) *Manager {
	lg := opt.Log
	if lg == nil {
		lg = log.Nop()
	}
	return &Manager{opt: opt, log: lg.With(log.Channel("editor"))}
}

func Clean(rel string) string {
	return path.Clean(filepath.ToSlash(rel))
}

func (m *Manager) Abs(rel string) string {
	return filepath.Join(m.opt.Root, filepath.FromSlash(rel))
}

func movedPath(p, oldRel, newRel string) (string, bool) {
	switch {
	case p == oldRel:
		return newRel, true
	case strings.HasPrefix(p, oldRel+"/"):
		return newRel + strings.TrimPrefix(p, oldRel), true
	}
	return "", false
}

func (m *Manager) Renamed(oldRel, newRel string) {
	oldRel, newRel = Clean(oldRel), Clean(newRel)
	if oldRel == newRel {
		return
	}
	for _, d := range m.docs {
		if np, ok := movedPath(d.Path, oldRel, newRel); ok {
			m.ClearRecovery(d.Path)
			d.Path = np
		}
	}
	for _, d := range m.scripts {
		if d.Path == oldRel+".meta" {
			m.clearScriptRecovery(d.Path)
			d.Path = newRel + ".meta"
			m.statScript(d)
			continue
		}
		if np, ok := movedPath(d.Path, oldRel, newRel); ok {
			m.clearScriptRecovery(d.Path)
			d.Path = np
			m.statScript(d)
		}
	}
	for _, d := range m.assets {
		if np, ok := movedPath(d.Path, oldRel, newRel); ok {
			d.Path = np
		}
	}
	// The stable document identity moves first; all presentations then update
	// their path indexes, including renames discovered through AssetID lookup.
	m.emit(Event{Kind: EvRenamed, Path: newRel, OldPath: oldRel})
}

func (m *Manager) Subscribe(fn func(Event)) func() {
	m.nextSub++
	id := m.nextSub
	m.subs = append(m.subs, subscriber{id: id, fn: fn})
	return func() {
		for i := range m.subs {
			if m.subs[i].id == id {
				m.subs = append(m.subs[:i], m.subs[i+1:]...)
				return
			}
		}
	}
}

func (m *Manager) emit(e Event) {
	for i := range m.subs {
		m.subs[i].fn(e)
	}
}

func (m *Manager) Docs() []*SceneDocument {
	return m.docs
}

func (m *Manager) Get(id ID) *SceneDocument {
	for _, d := range m.docs {
		if d.ID == id {
			return d
		}
	}
	return nil
}

func (m *Manager) Find(rel string) *SceneDocument {
	rel = Clean(rel)
	for _, d := range m.docs {
		if d.Path == rel {
			return d
		}
	}
	return nil
}

func (m *Manager) Active() *SceneDocument {
	if m.playDoc != nil && m.playDoc.ID == m.active {
		return m.playDoc
	}
	return m.Get(m.active)
}

func (m *Manager) Edit() *SceneDocument {
	return m.Get(m.active)
}

func (m *Manager) SetPlay(d *SceneDocument, w *engine.World) {
	m.playDoc, m.playWorld = d, w
	if d != nil {
		m.emit(Event{Kind: EvActivated, Doc: d.ID})
	}
}

func (m *Manager) ClearPlay() *SceneDocument {
	p := m.playDoc
	m.playDoc, m.playWorld = nil, nil
	if p != nil {
		m.emit(Event{Kind: EvActivated, Doc: p.ID})
	}
	return p
}

func (m *Manager) InPlay() bool {
	return m.playDoc != nil
}

func (m *Manager) PlayDoc() *SceneDocument {
	return m.playDoc
}

func (m *Manager) ActiveScene() *scene.Scene {
	if d := m.Active(); d != nil {
		return d.Scene()
	}
	return nil
}

func (m *Manager) ActiveWorld() *engine.World {
	if m.playDoc != nil && m.playDoc.ID == m.active {
		return m.playWorld
	}
	return nil
}

func (m *Manager) SetActive(id ID) {
	if m.Get(id) == nil || m.active == id {
		return
	}
	m.active = id
	m.emit(Event{Kind: EvActivated, Doc: id})
}

func (m *Manager) Dirty() bool {
	for _, d := range m.docs {
		if d.Dirty() {
			return true
		}
	}
	for _, d := range m.scripts {
		if d.Dirty() {
			return true
		}
	}
	for _, d := range m.assets {
		if d.Binding.Dirty != nil && d.Binding.Dirty() {
			return true
		}
	}
	return false
}

func (m *Manager) DirtyCount() int {
	n := 0
	for _, d := range m.docs {
		if d.Dirty() {
			n++
		}
	}
	for _, d := range m.scripts {
		if d.Dirty() {
			n++
		}
	}
	for _, d := range m.assets {
		if d.Binding.Dirty != nil && d.Binding.Dirty() {
			n++
		}
	}
	return n
}

func (m *Manager) load(rel string, data []byte) (*scene.Scene, error) {
	s, report, err := sceneio.Load(data, rel, m.opt.Types, m.opt.Resolver, sceneio.LoadOptions{KeepIDs: true})
	if err != nil {
		return nil, err
	}
	for _, w := range report.Warnings {
		m.log.Warn("scene warning", log.F("path", rel), log.F("line", w.Pos.Line), log.F("msg", w.Msg))
	}
	for _, u := range report.Unknown {
		m.log.Warn("unknown component preserved", log.F("path", rel), log.F("type", u))
	}
	return s, nil
}

func (m *Manager) newDoc(rel string, s *scene.Scene) *SceneDocument {
	m.next++
	d := &SceneDocument{
		ID:   m.next,
		Path: rel,
		View: ViewState{Camera: Camera{Zoom: 64}, Expanded: map[ids.NodeID]bool{}},
	}
	d.History = history.New(d)
	if m.opt.OnChange != nil {
		d.History.OnChange(m.opt.OnChange)
	}
	d.setScene(s)
	return d
}

func (m *Manager) add(d *SceneDocument, activate bool) {
	m.docs = append(m.docs, d)
	m.emit(Event{Kind: EvOpened, Doc: d.ID})
	if activate {
		m.SetActive(d.ID)
	}
}

func (m *Manager) stat(d *SceneDocument) {
	if st, err := os.Stat(m.Abs(d.Path)); err == nil {
		d.OnDisk = true
		d.DiskAt, d.DiskSize = st.ModTime(), st.Size()
	}
}

func (m *Manager) Open(rel string) (*SceneDocument, error) {
	rel = Clean(rel)
	prev := m.active
	if d := m.Find(rel); d != nil {
		m.noteBack(d, prev)
		m.SetActive(d.ID)
		return d, nil
	}
	start := time.Now()
	data, err := os.ReadFile(m.Abs(rel))
	if err != nil {
		return nil, errs.Wrap(err, errs.EditorDocument, "cannot read the scene").With(log.F("path", rel))
	}
	s, err := m.load(rel, data)
	if err != nil {
		return nil, err
	}
	d := m.newDoc(rel, s)
	d.Load = LoadStats{Took: time.Since(start), Nodes: max(0, len(s.PreOrder())-1)}
	m.stat(d)
	d.History.MarkSaved()
	m.noteBack(d, prev)
	m.add(d, true)
	return d, nil
}

func (m *Manager) OpenData(rel string, data []byte, dirty bool) (*SceneDocument, error) {
	return m.openData(rel, data, dirty, true)
}

func (m *Manager) openData(rel string, data []byte, dirty, activate bool) (*SceneDocument, error) {
	rel = Clean(rel)
	if m.Find(rel) != nil {
		return nil, errs.New(errs.EditorDocument, "scene is already open", log.F("path", rel))
	}
	s, err := m.load(rel, data)
	if err != nil {
		return nil, err
	}
	d := m.newDoc(rel, s)
	m.stat(d)
	d.forced = dirty
	m.add(d, activate)
	return d, nil
}

func (m *Manager) New(rel string, s *scene.Scene) *SceneDocument {
	d := m.newDoc(Clean(rel), s)
	d.forced = true
	m.add(d, true)
	return d
}

func (m *Manager) UniquePath(rel string) string {
	rel = Clean(rel)
	base := strings.TrimSuffix(rel, SceneExt)
	candidate := rel
	for n := 2; ; n++ {
		if m.Find(candidate) == nil {
			if _, err := os.Stat(m.Abs(candidate)); os.IsNotExist(err) {
				return candidate
			}
		}
		candidate = base + "-" + strconv.Itoa(n) + SceneExt
	}
}

func WriteAtomic(abs string, data []byte) error {
	return project.WriteAtomic(abs, data)
}

func (m *Manager) writeSource(write func() error) error {
	if m.opt.SourceWrite != nil {
		return m.opt.SourceWrite(write)
	}
	return write()
}

func encode(d *SceneDocument) []byte {
	if strings.HasSuffix(d.Path, PrefabExt) {
		return sceneio.SavePrefab(d.scene)
	}
	return sceneio.Save(d.scene)
}

func (m *Manager) Snapshot(d *SceneDocument) []byte { return m.guarded(d, encode) }

func (m *Manager) Save(id ID) error {
	d := m.Get(id)
	if d == nil {
		return errs.New(errs.EditorDocument, "no such document")
	}
	data := m.guarded(d, encode)
	if err := m.writeSource(func() error { return WriteAtomic(m.Abs(d.Path), data) }); err != nil {
		return errs.Wrap(err, errs.EditorDocument, "cannot save the scene").With(log.F("path", d.Path))
	}
	m.stat(d)
	d.forced = false
	d.removedAck = false
	d.History.MarkSaved()
	m.ClearRecovery(d.Path)
	m.emit(Event{Kind: EvSaved, Doc: id})
	return nil
}

func (m *Manager) SaveAs(id ID, rel string) error {
	d := m.Get(id)
	if d == nil {
		return errs.New(errs.EditorDocument, "no such document")
	}
	rel = Clean(strings.TrimSpace(rel))
	if rel == "." || rel == "" || strings.HasPrefix(rel, "..") {
		return errs.New(errs.EditorDocument, "path must stay inside the project", log.F("path", rel))
	}
	if !strings.HasSuffix(rel, SceneExt) {
		rel += SceneExt
	}
	if other := m.Find(rel); other != nil && other != d {
		return errs.New(errs.EditorDocument, "another open scene uses this path", log.F("path", rel))
	}
	old := d.Path
	d.Path = rel
	if err := m.Save(id); err != nil {
		d.Path = old
		return err
	}
	m.ClearRecovery(old)
	return nil
}

func (m *Manager) SaveAll() error {
	if err := m.CheckVolatileSave(); err != nil {
		return err
	}
	// Check structured inputs before saving a related dirty metadata buffer.
	for _, d := range m.assets {
		if d.Binding.BeforeSave != nil && d.Dirty() {
			if err := d.Binding.BeforeSave(); err != nil {
				return err
			}
		}
	}
	if err := m.CheckVolatileSave(); err != nil {
		return err
	}
	var first error
	pending := false
	record := func(err error) {
		if errors.Is(err, ErrSavePending) {
			pending = true
		} else if err != nil && first == nil {
			first = err
		}
	}
	for _, d := range m.docs {
		if !d.Dirty() {
			continue
		}
		record(m.Save(d.ID))
	}
	for _, d := range m.scripts {
		if !d.Dirty() {
			continue
		}
		record(m.SaveScript(d))
	}
	for _, d := range m.assets {
		if d.Binding.Save != nil && d.Binding.Dirty != nil && d.Binding.Dirty() {
			if err := m.CheckVolatileSave(); err != nil {
				record(err)
				break
			}
			record(d.Binding.Save())
		}
	}
	if err := m.CheckVolatileSave(); err != nil {
		record(err)
	}
	if first == nil && pending {
		return ErrSavePending
	}
	return first
}

func (m *Manager) Close(id ID, force bool) error {
	for i, d := range m.docs {
		if d.ID != id {
			continue
		}
		if d.Dirty() && !force {
			return errs.New(errs.EditorDocument, "scene has unsaved changes", log.F("path", d.Path))
		}
		if force {
			m.ClearRecovery(d.Path)
		}
		target, hasTarget := m.BackTarget(id)
		m.forget(id)
		m.docs = append(m.docs[:i], m.docs[i+1:]...)
		for _, asset := range m.assets {
			if asset.Scene == d {
				asset.Scene = nil
			}
		}
		if m.active == id {
			m.active = 0
			switch {
			case hasTarget:
				m.SetActive(target)
			case len(m.docs) > 0:
				m.SetActive(m.docs[len(m.docs)-1].ID)
			}
		}
		m.emit(Event{Kind: EvClosed, Doc: id})
		return nil
	}
	return errs.New(errs.EditorDocument, "no such document")
}

func (d *SceneDocument) IsPrefab() bool {
	return strings.HasSuffix(d.Path, PrefabExt)
}

func (d *SceneDocument) Label() string {
	base := path.Base(d.Path)
	if d.IsPrefab() {
		return i18n.Tr("editor.document.trimsuffix_prefab", "{trimsuffix} (Prefab)", i18n.Str("trimsuffix", strings.TrimSuffix(base, PrefabExt)))
	}
	return base
}

const DirtyMark = "•"

func (d *SceneDocument) Caption() string {
	if d.Dirty() {
		return d.Label() + " " + DirtyMark
	}
	return d.Label()
}

func (m *Manager) Origin(id ID) (ID, bool) {
	o, ok := m.origin[id]
	return o, ok && m.Get(o) != nil
}

func (m *Manager) BackTarget(id ID) (ID, bool) {
	if o, ok := m.Origin(id); ok {
		return o, true
	}
	b, ok := m.back[id]
	return b, ok && m.Get(b) != nil
}

func (m *Manager) Back(id ID) bool {
	t, ok := m.BackTarget(id)
	if !ok {
		return false
	}
	m.SetActive(t)
	return true
}

func (m *Manager) Chain(id ID) []*SceneDocument {
	var out []*SceneDocument
	seen := map[ID]bool{}
	for cur := id; cur != 0 && !seen[cur]; {
		seen[cur] = true
		d := m.Get(cur)
		if d == nil {
			break
		}
		out = append([]*SceneDocument{d}, out...)
		o, ok := m.origin[cur]
		if !ok {
			break
		}
		cur = o
	}
	return out
}

func (m *Manager) PlaySource(id ID) (*SceneDocument, bool) {
	d := m.Get(id)
	if d == nil {
		return nil, false
	}
	if !d.IsPrefab() {
		return d, true
	}
	if chain := m.Chain(id); len(chain) > 0 && !chain[0].IsPrefab() {
		return chain[0], true
	}
	return nil, false
}

func (m *Manager) Pending(id ID) []*SceneDocument {
	var out []*SceneDocument
	for _, d := range m.docs {
		if d.ID == id || !d.IsPrefab() || !d.Dirty() {
			continue
		}
		if t, ok := m.BackTarget(d.ID); ok && t == id {
			out = append(out, d)
		}
	}
	return out
}

func (m *Manager) remember(d *SceneDocument, from ID) {
	if !d.IsPrefab() || from == 0 || from == d.ID || m.Get(from) == nil {
		return
	}
	for _, c := range m.Chain(from) {
		if c.ID == d.ID {
			return
		}
	}
	if m.origin == nil {
		m.origin = map[ID]ID{}
	}
	m.origin[d.ID] = from
}

func (m *Manager) noteBack(d *SceneDocument, prev ID) {
	if !d.IsPrefab() || prev == 0 || prev == d.ID {
		return
	}
	if m.back == nil {
		m.back = map[ID]ID{}
	}
	m.back[d.ID] = prev
}

func (m *Manager) OpenFrom(rel string, from ID) (*SceneDocument, error) {
	d, err := m.Open(rel)
	if err != nil {
		return nil, err
	}
	m.remember(d, from)
	return d, nil
}

func (m *Manager) forget(id ID) {
	target, has := m.BackTarget(id)
	for _, links := range []map[ID]ID{m.origin, m.back} {
		for child, o := range links {
			if o != id {
				continue
			}
			if has && target != child {
				links[child] = target
			} else {
				delete(links, child)
			}
		}
	}
	delete(m.origin, id)
	delete(m.back, id)
}

func (m *Manager) Reload(id ID) error {
	d := m.Get(id)
	if d == nil {
		return errs.New(errs.EditorDocument, "no such document")
	}
	data, err := os.ReadFile(m.Abs(d.Path))
	if err != nil {
		return errs.Wrap(err, errs.EditorDocument, "cannot read the scene").With(log.F("path", d.Path))
	}
	s, err := m.load(d.Path, data)
	if err != nil {
		return err
	}
	d.setScene(s)
	d.History.Clear()
	d.forced = false
	m.stat(d)
	m.ClearRecovery(d.Path)
	m.emit(Event{Kind: EvReloaded, Doc: id})
	return nil
}

func (m *Manager) KeepMine(id ID) {
	if d := m.Get(id); d != nil {
		d.forced = true
	}
}

func (m *Manager) CheckExternal() ([]ID, []Conflict) {
	var reloaded []ID
	var conflicts []Conflict
	for _, d := range m.docs {
		if !d.OnDisk {
			continue
		}
		st, err := os.Stat(m.Abs(d.Path))
		if err != nil {
			if os.IsNotExist(err) && !d.removedAck {
				d.removedAck = true
				conflicts = append(conflicts, Conflict{Doc: d.ID, Path: d.Path, Removed: true})
			}
			continue
		}
		if st.ModTime().Equal(d.DiskAt) && st.Size() == d.DiskSize {
			continue
		}
		if !d.Dirty() {
			if err := m.Reload(d.ID); err != nil {
				m.log.Warn("external change could not be loaded yet", log.F("path", d.Path), log.F("err", err.Error()))
				continue
			}
			reloaded = append(reloaded, d.ID)
			continue
		}
		d.DiskAt, d.DiskSize = st.ModTime(), st.Size()
		conflicts = append(conflicts, Conflict{Doc: d.ID, Path: d.Path})
	}
	return reloaded, conflicts
}

func (m *Manager) recoveryDir() string {
	return filepath.Join(m.opt.Root, filepath.FromSlash(RecoveryDir))
}

func (m *Manager) recoveryFile(rel string) string {
	return filepath.Join(m.recoveryDir(), hex.EncodeToString([]byte(rel))+SceneExt)
}

func (m *Manager) WriteRecovery() (int, error) {
	n := 0
	var first error
	for _, d := range m.docs {
		if !d.Dirty() {
			m.ClearRecovery(d.Path)
			continue
		}
		name, data := m.recoveryFile(d.Path), m.guarded(d, recoveryBytes)
		if err := m.writeRecoveryFile(name, data); err != nil {
			if first == nil {
				first = errs.Wrap(err, errs.EditorDocument, "cannot write the recovery file")
			}
			continue
		}
		n++
	}
	for _, d := range m.assets {
		if d.Binding.Recovery == nil || !d.Dirty() {
			continue
		}
		if err := m.WriteAssetRecovery(d); err != nil {
			if first == nil {
				first = err
			}
		} else {
			n++
		}
	}
	sn, err := m.writeScriptRecovery()
	if first == nil {
		first = err
	}
	return n + sn, first
}

func (m *Manager) FindRecovery() []Recovery {
	entries, err := os.ReadDir(m.recoveryDir())
	if err != nil {
		return nil
	}
	var out []Recovery
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, SceneExt) {
			continue
		}
		raw, err := hex.DecodeString(strings.TrimSuffix(name, SceneExt))
		if err != nil {
			continue
		}
		rel := string(raw)
		if m.Find(rel) != nil {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		full := filepath.Join(m.recoveryDir(), name)
		if st, err := os.Stat(m.Abs(rel)); err == nil && !st.ModTime().Before(info.ModTime()) {
			// A newer source does not authorize deleting the older recovery.
			continue
		}
		out = append(out, Recovery{Path: rel, File: full, Time: info.ModTime()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func (m *Manager) RestoreRecovery(r Recovery) (*SceneDocument, error) {
	data, err := os.ReadFile(r.File)
	if err != nil {
		return nil, errs.Wrap(err, errs.EditorDocument, "cannot read the recovery file")
	}
	// Recovering another scene must not steal the scene already selected in
	// the editor. With no active scene, keep the recovered scene visible.
	d, err := m.openData(r.Path, data, true, m.Active() == nil)
	if err == nil && filepath.Clean(r.File) == filepath.Clean(m.recoveryFile(r.Path)) {
		m.trackRecovery(r.File, data)
	}
	return d, err
}

func (m *Manager) DiscardRecovery(r Recovery) {
	os.Remove(r.File)
}

func (m *Manager) ClearRecovery(rel string) {
	if err := m.discardOwnedRecovery(m.recoveryFile(rel)); err != nil {
		m.log.Warn("cannot clear changed recovery file", log.F("path", rel), log.F("err", err.Error()))
	}
}

// DiscardOpenRecovery removes only recovery snapshots belonging to the dirty
// documents the user is discarding. Unopened recovery files and an asset's
// unresolved prior recovery are not covered by that confirmation.
func (m *Manager) DiscardOpenRecovery() error {
	var failures []error
	remove := func(name string) {
		if err := m.discardOwnedRecovery(name); err != nil {
			failures = append(failures, errs.Wrap(err, errs.EditorDocument, "cannot discard the recovery file").With(log.F("path", name)))
		}
	}
	for _, d := range m.docs {
		if d.Dirty() {
			remove(m.recoveryFile(d.Path))
		}
	}
	for _, d := range m.scripts {
		if d.Dirty() {
			remove(m.scriptRecoveryFile(d.Path))
		}
	}
	for _, d := range m.assets {
		if d.Binding.Recovery != nil && d.Binding.Dirty != nil && d.Binding.Dirty() && (d.Binding.PendingRecovery == nil || !d.Binding.PendingRecovery()) {
			remove(m.assetRecoveryPath(d))
		}
	}
	return errors.Join(failures...)
}

func (d *SceneDocument) PreviewBytes() []byte { return encode(d) }
