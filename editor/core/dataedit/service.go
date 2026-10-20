package dataedit

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"strings"
	"unsafe"

	"aego/asset"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/data"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/scene/mutate"
	"aego/schema"
)

type change struct {
	id        ids.AssetID
	field     *schema.FieldDescriptor
	before    schema.Value
	ptr       unsafe.Pointer
	inherited bool
}

type Service struct {
	ctx       *context.Editor
	docs      map[ids.AssetID]*Doc
	tables    map[ids.AssetID]*Table
	inspected ids.AssetID
	changes   []change
	types     map[ids.AssetID]string
	typesRev  uint64
	lib       *data.Library
	libSrc    asset.DataSource
	OnLive    func()
}

func New(ctx *context.Editor) *Service {
	return &Service{ctx: ctx, docs: map[ids.AssetID]*Doc{}, tables: map[ids.AssetID]*Table{}, types: map[ids.AssetID]string{}}
}

func (s *Service) reg() *schema.Registry {
	return s.ctx.Types.Schema()
}

func (s *Service) Inspect(id ids.AssetID) {
	s.inspected = id
}

func (s *Service) Clear() {
	s.inspected = ids.AssetID{}
}

func (s *Service) Inspected() ids.AssetID {
	if s.inspected.IsZero() || s.ctx.Pipeline == nil {
		return ids.AssetID{}
	}
	if _, ok := s.ctx.Pipeline.PathOf(s.inspected); !ok {
		s.inspected = ids.AssetID{}
	}
	return s.inspected
}

func (s *Service) InPlay() bool {
	return s.ctx.Docs != nil && s.ctx.Docs.InPlay()
}

func (s *Service) source() (asset.DataSource, bool) {
	if s.ctx.Loader == nil {
		return nil, false
	}
	src, ok := any(s.ctx.Loader).(asset.DataSource)
	return src, ok
}

func (s *Service) Library() (*data.Library, error) {
	src, ok := s.source()
	if !ok {
		return nil, data.ErrNoSource
	}
	if s.lib == nil || s.libSrc != src {
		s.lib, s.libSrc = data.NewLibrary(s.reg(), src), src
	}
	return s.lib, nil
}

func (s *Service) noPipeline() error {
	return errs.New(errs.EditorDocument, "the asset pipeline is not available")
}

func (s *Service) file(id ids.AssetID) (string, []byte, error) {
	pl := s.ctx.Pipeline
	if pl == nil {
		return "", nil, s.noPipeline()
	}
	rel, ok := pl.PathOf(id)
	if !ok {
		return "", nil, errs.New(errs.EditorDocument, "asset "+id.Short()+" is not in the project")
	}
	raw, err := os.ReadFile(pl.Abs(rel))
	if err != nil {
		return "", nil, errs.Wrap(err, errs.EditorDocument, "cannot read the data asset").With(log.F("path", rel))
	}
	return rel, raw, nil
}

func (s *Service) env() mutate.Env {
	env := mutate.Env{Types: s.ctx.Types}
	if pl := s.ctx.Pipeline; pl != nil {
		env.Lookup = pl.Lookup
		env.Assets = func(id ids.AssetID) bool {
			_, ok := pl.PathOf(id)
			return ok || asset.IsBuiltin(id)
		}
	}
	return env
}

func (s *Service) Doc(id ids.AssetID) (*Doc, error) {
	d := s.docs[id]
	if d != nil && d.rev == s.ctx.AssetRev {
		return d, nil
	}
	rel, raw, err := s.file(id)
	if err != nil {
		return nil, err
	}
	if d != nil && d.Dirty() {
		s.conflict(rel, raw, d.disk, &d.conflict)
		d.Path, d.rev = rel, s.ctx.AssetRev
		return d, d.load()
	}
	if d != nil && bytes.Equal(raw, d.disk) {
		d.Path, d.rev = rel, s.ctx.AssetRev
		return d, d.load()
	}
	lib, err := s.Library()
	if err != nil {
		return nil, err
	}
	nd, err := NewDoc(s.env(), lib, id, rel, raw)
	if err != nil {
		return nil, err
	}
	nd.disk, nd.rev = raw, s.ctx.AssetRev
	s.docs[id] = nd
	return nd, nil
}

func (s *Service) Table(id ids.AssetID) (*Table, error) {
	t := s.tables[id]
	if t != nil && t.rev == s.ctx.AssetRev {
		return t, nil
	}
	rel, raw, err := s.file(id)
	if err != nil {
		return nil, err
	}
	if t != nil && (t.Dirty() || bytes.Equal(raw, t.disk)) {
		if t.Dirty() {
			s.conflict(rel, raw, t.disk, &t.conflict)
		}
		t.Path, t.rev = rel, s.ctx.AssetRev
		return t, nil
	}
	nt, err := NewTable(s.env(), id, rel, raw)
	if err != nil {
		return nil, err
	}
	nt.disk, nt.rev = raw, s.ctx.AssetRev
	s.tables[id] = nt
	return nt, nil
}

func (s *Service) conflict(rel string, raw, disk []byte, seen *[]byte) {
	if bytes.Equal(raw, disk) || bytes.Equal(raw, *seen) {
		return
	}
	*seen = raw
	s.ctx.Notify(context.LevelWarn, rel+" changed on disk while it has unsaved changes: Save keeps yours, Revert loads the disk version")
}

func (s *Service) RevertDoc(id ids.AssetID) {
	delete(s.docs, id)
}

func (s *Service) RevertTable(id ids.AssetID) {
	delete(s.tables, id)
}

func (s *Service) save(id ids.AssetID, rel string, b []byte, done func([]byte)) error {
	pl := s.ctx.Pipeline
	if pl == nil {
		return s.noPipeline()
	}
	if err := document.WriteAtomic(pl.Abs(rel), b); err != nil {
		return errs.Wrap(err, errs.EditorDocument, "cannot save the data asset").With(log.F("path", rel))
	}
	done(b)
	batch, err := pl.Reimport(id)
	s.ctx.ApplyBatch(batch)
	if d := s.docs[id]; d != nil {
		d.rev = s.ctx.AssetRev
	}
	if t := s.tables[id]; t != nil {
		t.rev = s.ctx.AssetRev
	}
	return err
}

func (s *Service) playLocked() error {
	if s.InPlay() {
		return errs.New(errs.EditorCommand, "data assets are saved after Play")
	}
	return nil
}

func (s *Service) SaveDoc(d *Doc) error {
	if err := s.playLocked(); err != nil {
		return err
	}
	return s.save(d.ID, d.Path, d.Bytes(), func(b []byte) {
		d.MarkSaved()
		d.disk = b
	})
}

func (s *Service) SaveTable(t *Table) error {
	if err := s.playLocked(); err != nil {
		return err
	}
	return s.save(t.ID, t.Path, t.Bytes(), func(b []byte) {
		t.MarkSaved()
		t.disk = b
	})
}

func csvPath(rel string) string {
	return strings.TrimSuffix(rel, path.Ext(rel)) + ".csv"
}

func (s *Service) ExportCSV(t *Table) (string, error) {
	if s.ctx.Pipeline == nil {
		return "", s.noPipeline()
	}
	b, err := t.ExportCSV()
	if err != nil {
		return "", err
	}
	rel := csvPath(t.Path)
	if err := document.WriteAtomic(s.ctx.Pipeline.Abs(rel), b); err != nil {
		return "", errs.Wrap(err, errs.EditorDocument, "cannot write "+rel)
	}
	return rel, nil
}

func (s *Service) ImportCSV(t *Table) (string, error) {
	if s.ctx.Pipeline == nil {
		return "", s.noPipeline()
	}
	rel := csvPath(t.Path)
	b, err := os.ReadFile(s.ctx.Pipeline.Abs(rel))
	if err != nil {
		return rel, errs.Wrap(err, errs.EditorDocument, "cannot read "+rel)
	}
	return rel, t.ImportCSV(b)
}

func (s *Service) AssetLabel(id ids.AssetID) string {
	if s.ctx.Pipeline != nil {
		if e, ok := s.ctx.Pipeline.Entry(id); ok && !e.Missing {
			return path.Base(e.Name)
		}
	}
	return id.Short()
}

func (s *Service) TypeOf(id ids.AssetID) (string, bool) {
	if s.typesRev != s.ctx.AssetRev {
		clear(s.types)
		s.typesRev = s.ctx.AssetRev
	}
	t, ok := s.types[id]
	if !ok {
		t = s.readType(id)
		s.types[id] = t
	}
	return t, t != ""
}

func (s *Service) readType(id ids.AssetID) string {
	src, ok := s.source()
	if !ok {
		return ""
	}
	if raw, ok := src.Data(id); ok {
		if d, err := data.ParseDoc(raw, id.String()); err == nil {
			return d.Type
		}
	}
	if raw, ok := src.Table(id); ok {
		if t, err := data.ParseTable(raw, id.String()); err == nil {
			return t.Type
		}
	}
	return ""
}

func (s *Service) LiveRecord(id ids.AssetID) (unsafe.Pointer, error) {
	lib, err := s.Library()
	if err != nil {
		return nil, err
	}
	rec, err := lib.Record(id)
	if err != nil {
		return nil, err
	}
	return rec.Ptr, nil
}

func (s *Service) LiveSet(id ids.AssetID, fd *schema.FieldDescriptor, v schema.Value) error {
	if !fd.Flags.Has(schema.FlagLive) {
		return errs.New(errs.EditorCommand, fd.Name+" is not a live field, change it after Play")
	}
	lib, err := s.Library()
	if err != nil {
		return err
	}
	rec, err := lib.Record(id)
	if err != nil {
		return err
	}
	heirs := s.heirs(lib, rec, fd, nil, 0)
	if err := s.patch(rec, fd, v, false); err != nil {
		return err
	}
	for _, h := range heirs {
		if err := s.patch(h, fd, v, true); err != nil {
			return err
		}
	}
	if s.OnLive != nil {
		s.OnLive()
	}
	return nil
}

func (s *Service) heirs(lib *data.Library, rec *data.Record, fd *schema.FieldDescriptor, out []*data.Record, depth int) []*data.Record {
	if depth >= data.MaxDepth || s.ctx.Pipeline == nil {
		return out
	}
	for _, dep := range s.ctx.Pipeline.Dependents(rec.ID) {
		child, err := lib.Record(dep)
		if err != nil || child.Base != rec.ID || child.Type != rec.Type || s.overrides(dep, fd.Name) {
			continue
		}
		out = append(out, child)
		out = s.heirs(lib, child, fd, out, depth+1)
	}
	return out
}

func (s *Service) overrides(id ids.AssetID, name string) bool {
	src, ok := s.source()
	if !ok {
		return false
	}
	raw, ok := src.Data(id)
	if !ok {
		return false
	}
	d, err := data.ParseDoc(raw, id.String())
	if err != nil {
		return false
	}
	for _, f := range d.Fields {
		if f.Name == name {
			return true
		}
	}
	return false
}

func (s *Service) touched(id ids.AssetID, fd *schema.FieldDescriptor) bool {
	for _, c := range s.changes {
		if c.id == id && c.field == fd {
			return true
		}
	}
	return false
}

func (s *Service) patch(rec *data.Record, fd *schema.FieldDescriptor, v schema.Value, inherited bool) error {
	if !s.touched(rec.ID, fd) {
		var before schema.Value
		fd.Get(rec.Ptr, &before)
		s.changes = append(s.changes, change{id: rec.ID, field: fd, before: before, ptr: rec.Ptr, inherited: inherited})
	}
	return fd.Set(rec.Ptr, v)
}

func (s *Service) LiveChanges() int {
	return len(s.changes)
}

func (s *Service) PlayStopped() {
	if len(s.changes) == 0 {
		return
	}
	list := s.changes
	s.changes = nil
	own := map[ids.AssetID]bool{}
	fields := 0
	for _, c := range list {
		if !c.inherited {
			own[c.id] = true
			fields++
		}
	}
	s.ctx.Ask(context.Question{
		Title:   "Live data changes",
		Message: fmt.Sprintf("%d field(s) in %d data asset(s) were tuned during Play. Keep them in the assets?", fields, len(own)),
		Buttons: []string{"Keep", "Discard"},
		Then: func(choice int) {
			if choice == 0 {
				s.keep(list)
				return
			}
			s.discard(list)
		},
	})
}

func (s *Service) discard(list []change) {
	for i := len(list) - 1; i >= 0; i-- {
		c := list[i]
		c.field.Set(c.ptr, c.before)
	}
}

type kept struct {
	fd *schema.FieldDescriptor
	v  schema.Value
}

func (s *Service) keep(list []change) {
	var order []ids.AssetID
	byID := map[ids.AssetID][]kept{}
	for _, c := range list {
		if c.inherited {
			continue
		}
		var v schema.Value
		c.field.Get(c.ptr, &v)
		if _, ok := byID[c.id]; !ok {
			order = append(order, c.id)
		}
		byID[c.id] = append(byID[c.id], kept{fd: c.field, v: v})
	}
	s.discard(list)
	for _, id := range order {
		d, err := s.Doc(id)
		if err != nil {
			s.ctx.Notify(context.LevelError, err.Error())
			continue
		}
		for i, k := range byID[id] {
			if err := d.Set(k.fd, k.v, i == 0); err != nil {
				s.ctx.Notify(context.LevelError, err.Error())
			}
		}
		if err := s.SaveDoc(d); err != nil {
			s.ctx.Notify(context.LevelError, err.Error())
			continue
		}
		s.ctx.Notify(context.LevelInfo, "Kept the live changes in "+d.Path)
	}
}
