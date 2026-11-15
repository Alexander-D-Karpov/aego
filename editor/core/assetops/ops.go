package assetops

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"aego/asset/pipeline"
	"aego/core/errs"
	"aego/core/ids"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/engine/i18n"
	"aego/project/refactor"
	"aego/project/rename"
)

const (
	PrefabExt = ".aeprefab"
	opsLimit  = 64
)

type FileOp struct {
	Dir     bool
	ID      ids.AssetID
	From    string
	To      string
	Created bool
	Source  []byte
	Meta    []byte
	// A source group is one durable Pipeline transaction, including metadata.
	// Only CreateSources constructs it; the existing asset undo stack owns it.
	createdSources []pipeline.SourceEdit
	createdAssets  map[string]ids.AssetID
}

type Ops struct {
	undo  [][]FileOp
	redo  [][]FileOp
	group []FileOp
}

func (o *Ops) CanUndo() bool {
	return len(o.undo) > 0
}

func (o *Ops) CanRedo() bool {
	return len(o.redo) > 0
}

func (o *Ops) Apply(ctx *context.Editor, ops []FileOp) error {
	done, err := runOps(ctx, ops, false)
	if len(done) > 0 {
		o.record(done)
	}
	return err
}

func (o *Ops) record(step []FileOp) {
	if len(step) == 0 {
		return
	}
	if o.group != nil {
		o.group = append(o.group, step...)
		return
	}
	o.undo = append(o.undo, step)
	if over := len(o.undo) - opsLimit; over > 0 {
		o.undo = append(o.undo[:0], o.undo[over:]...)
	}
	o.redo = o.redo[:0]
}

// Transaction groups existing file operations into one asset undo entry.
// Failed multi-file creation restores the files created earlier in the group.
func (o *Ops) Transaction(ctx *context.Editor, fn func() error) error {
	if o.group != nil {
		return fn()
	}
	o.group = make([]FileOp, 0, 2)
	err := fn()
	step := o.group
	o.group = nil
	if err != nil {
		_, rollback := runOps(ctx, step, true)
		return errors.Join(err, rollback)
	}
	o.record(step)
	return nil
}

func (o *Ops) Undo(ctx *context.Editor) error {
	n := len(o.undo)
	if n == 0 {
		return nil
	}
	step := o.undo[n-1]
	o.undo = o.undo[:n-1]
	if done, err := runOps(ctx, step, true); err != nil {
		if len(done) == 0 {
			o.undo = append(o.undo, step)
		} else {
			o.undo, o.redo = o.undo[:0], o.redo[:0]
		}
		return err
	}
	o.redo = append(o.redo, step)
	return nil
}

func (o *Ops) Redo(ctx *context.Editor) error {
	n := len(o.redo)
	if n == 0 {
		return nil
	}
	step := o.redo[n-1]
	o.redo = o.redo[:n-1]
	if done, err := runOps(ctx, step, false); err != nil {
		if len(done) == 0 {
			o.redo = append(o.redo, step)
		} else {
			o.undo, o.redo = o.undo[:0], o.redo[:0]
		}
		return err
	}
	o.undo = append(o.undo, step)
	return nil
}

func runOps(ctx *context.Editor, ops []FileOp, back bool) ([]FileOp, error) {
	done := make([]FileOp, 0, len(ops))
	for _, op := range ops {
		if len(op.createdSources) > 0 {
			if err := op.checkCreatedSources(ctx, back); err != nil {
				return nil, err
			}
		}
		if op.Created {
			if err := op.checkCreated(ctx, back); err != nil {
				return nil, err
			}
		}
	}
	for i := range ops {
		op := ops[i]
		if back {
			op = ops[len(ops)-1-i]
		}
		if err := op.apply(ctx, back); err != nil {
			return done, err
		}
		done = append(done, op)
	}
	return done, nil
}

func (f FileOp) apply(ctx *context.Editor, back bool) error {
	if len(f.createdSources) > 0 {
		return f.applyCreatedSources(ctx, back)
	}
	if f.Created {
		return f.created(ctx, back)
	}
	from, to := f.From, f.To
	if back {
		from, to = to, from
	}
	if f.Dir {
		return moveDir(ctx, from, to)
	}
	return MoveFile(ctx, f.ID, from, to)
}

// CreateFile records source and metadata in the same asset history as moves.
// Undo refuses to remove a file that acquired saved or unsaved edits meanwhile.
func (o *Ops) CreateFile(ctx *context.Editor, rel string, data []byte) (ids.AssetID, error) {
	id, err := CreateFile(ctx, rel, data)
	if id.IsZero() {
		return id, err
	}
	meta, readErr := os.ReadFile(ctx.Pipeline.Abs(rel) + pipeline.MetaSuffix)
	if readErr != nil {
		return id, readErr
	}
	o.record([]FileOp{{ID: id, To: rel, Created: true, Source: bytes.Clone(data), Meta: meta}})
	return id, err
}

func (f FileOp) checkCreated(ctx *context.Editor, back bool) error {
	abs := ctx.Pipeline.Abs(f.To)
	if back {
		if ctx.Docs != nil {
			if d := ctx.Docs.Script(f.To); d != nil && d.Dirty() {
				return errs.New(errs.EditorDocument, "cannot undo creation while the source has unsaved edits: "+f.To)
			}
		}
		source, err := os.ReadFile(abs)
		if err != nil {
			return err
		}
		meta, err := os.ReadFile(abs + pipeline.MetaSuffix)
		if err != nil {
			return err
		}
		if !bytes.Equal(source, f.Source) || !bytes.Equal(meta, f.Meta) {
			return errs.New(errs.EditorDocument, "cannot undo creation because the source or metadata changed: "+f.To)
		}
		if id, ok := ctx.Pipeline.Lookup(f.To); !ok || id != f.ID {
			return errs.New(errs.EditorDocument, "cannot undo creation: the asset identity changed")
		}
		return nil
	}
	if Exists(ctx, f.To) || Exists(ctx, f.To+pipeline.MetaSuffix) {
		return errs.New(errs.EditorDocument, "cannot redo creation: "+f.To+" already exists")
	}
	return nil
}

func (f FileOp) created(ctx *context.Editor, back bool) error {
	abs := ctx.Pipeline.Abs(f.To)
	if err := f.checkCreated(ctx, back); err != nil {
		return err
	}
	if back {
		batch, err := ctx.Pipeline.Delete(f.ID)
		ctx.ApplyBatch(batch)
		if err == nil && ctx.Docs != nil {
			if d := ctx.Docs.Script(f.To); d != nil {
				d.OnDisk = false
			}
		}
		return err
	}
	if Exists(ctx, f.To) || Exists(ctx, f.To+pipeline.MetaSuffix) {
		return errs.New(errs.EditorDocument, "cannot redo creation: "+f.To+" already exists")
	}
	if err := document.WriteAtomic(abs+pipeline.MetaSuffix, f.Meta); err != nil {
		return err
	}
	if err := document.WriteAtomic(abs, f.Source); err != nil {
		os.Remove(abs + pipeline.MetaSuffix)
		return err
	}
	if _, _, err := ctx.Pipeline.Scan(); err != nil {
		return err
	}
	batch, err := ctx.Pipeline.Import(f.ID)
	ctx.ApplyBatch(batch)
	if err == nil && ctx.Docs != nil {
		if d := ctx.Docs.Script(f.To); d != nil {
			ctx.Docs.KeepScript(d)
		}
	}
	return err
}

func MoveFile(ctx *context.Editor, id ids.AssetID, from, to string) error {
	if from == to {
		return nil
	}
	if Exists(ctx, to) {
		return errs.New(errs.EditorDocument, to+" already exists")
	}
	if err := ctx.Pipeline.Move(id, to); err != nil {
		return err
	}
	renamed(ctx, from, to)
	if path.Ext(to) == document.ScriptExt {
		retarget(ctx, from, to)
	}
	return nil
}

func moveDir(ctx *context.Editor, from, to string) error {
	if from == to {
		return nil
	}
	if Under(to, from) {
		return errs.New(errs.EditorDocument, "cannot move "+from+" into itself")
	}
	if err := ctx.Pipeline.MoveDir(from, to); err != nil {
		return err
	}
	renamed(ctx, from, to)
	for _, a := range ctx.Pipeline.Assets() {
		if Under(a.Path, to) && path.Ext(a.Path) == document.ScriptExt {
			retarget(ctx, from+strings.TrimPrefix(a.Path, to), a.Path)
		}
	}
	return nil
}

func renamed(ctx *context.Editor, from, to string) {
	ctx.Docs.Renamed(from, to)
	if ctx.Code != nil {
		ctx.Code.Renamed(from, to)
	}
}

func retarget(ctx *context.Editor, from, to string) {
	was, now := PackageOf(ctx, path.Dir(from)), PackageOf(ctx, path.Dir(to))
	if was == now {
		return
	}
	if err := setPackage(ctx, to, now); err != nil {
		ctx.Notify(context.LevelError, i18n.Note("editor.assetops.cannot_update_the_package_of", "cannot update the package of {to}: {error}", i18n.Str("to", to), i18n.Str("error", err.Error())))
		return
	}
	ctx.Notify(context.LevelWarn, i18n.Note("editor.assetops.to_is_now_in_package", "{to} is now in package {now} (was {was}), fix the imports that used it", i18n.Str("to", path.Base(to)), i18n.Str("now", now), i18n.Str("was", was)))
}

func openScript(ctx *context.Editor, rel string) bool {
	for _, d := range ctx.Docs.Scripts() {
		if d.Path == rel {
			return true
		}
	}
	return false
}

func setPackage(ctx *context.Editor, rel, pkg string) error {
	wasOpen := openScript(ctx, rel)
	d, err := ctx.Docs.OpenScript(rel)
	if err != nil {
		return err
	}
	c := d.Code
	if a, b, ok := refactor.PackageSpan(c.Bytes()); ok {
		c.ReplaceRanges([][2]int{{a, b}}, [][]byte{[]byte(pkg)})
	} else {
		c.ReplaceRanges([][2]int{{0, 0}}, [][]byte{[]byte("package " + pkg + "\n\n")})
	}
	err = ctx.Docs.SaveScript(d)
	if !wasOpen {
		ctx.Docs.CloseScript(d)
	}
	return err
}

func Exists(ctx *context.Editor, rel string) bool {
	_, err := os.Stat(ctx.Pipeline.Abs(rel))
	return err == nil
}

func Under(p, dir string) bool {
	return strings.HasPrefix(p, dir+"/")
}

func Stem(p string) string {
	b := path.Base(p)
	return strings.TrimSuffix(b, path.Ext(b))
}

func ValidName(s string) bool {
	return rename.ValidName(s)
}

func ValidIdent(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '_', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

func UniqueFile(ctx *context.Editor, dir, stem, ext string) string {
	for i := 1; ; i++ {
		name := stem
		if i > 1 {
			name += " " + strconv.Itoa(i)
		}
		if rel := path.Join(dir, name+ext); !Exists(ctx, rel) {
			return rel
		}
	}
}

func CreateFile(ctx *context.Editor, rel string, data []byte) (ids.AssetID, error) {
	clean := path.Clean(rel)
	if clean != rel || path.IsAbs(rel) || filepath.IsAbs(rel) || strings.ContainsAny(rel, "\\\x00") || !(Under(rel, "assets") || Under(rel, "scenes") || Under(rel, "scripts") || Under(rel, "prefabs")) {
		return ids.AssetID{}, errs.New(errs.EditorDocument, "asset path must be a clean path under assets, scenes, scripts or prefabs")
	}
	if Exists(ctx, rel) {
		return ids.AssetID{}, errs.New(errs.EditorDocument, rel+" already exists")
	}
	abs := ctx.Pipeline.Abs(rel)
	if err := document.WriteAtomic(abs, data); err != nil {
		return ids.AssetID{}, errs.Wrap(err, errs.VFSIO, "cannot write "+rel)
	}
	if _, _, err := ctx.Pipeline.Scan(); err != nil {
		return ids.AssetID{}, err
	}
	id, ok := ctx.Pipeline.Lookup(rel)
	if !ok {
		os.Remove(abs)
		return ids.AssetID{}, errs.New(errs.EditorDocument, rel+" has no importer or is outside assets, scenes, scripts and prefabs")
	}
	batch, err := ctx.Pipeline.Import(id)
	ctx.ApplyBatch(batch)
	if err == nil && batch != nil {
		if cause := batch.Failed[id]; cause != nil {
			return id, fmt.Errorf("%s was created, but import failed: %w", rel, cause)
		}
		if slices.Contains(batch.Retry, id) {
			return id, fmt.Errorf("%s was created, but import must be retried because the source is unstable", rel)
		}
	}
	return id, err
}

func ScriptsRel(dir string) (string, bool) {
	switch {
	case dir == "scripts":
		return "", true
	case strings.HasPrefix(dir, "scripts/"):
		return dir[len("scripts/"):], true
	}
	return "", false
}

func PackageOf(ctx *context.Editor, dir string) string {
	return refactor.PackageOf(ctx.Project, dir)
}

func FileStem(name string) string {
	var b strings.Builder
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c >= 'A' && c <= 'Z' {
			if i > 0 && name[i-1] != '_' && !(name[i-1] >= 'A' && name[i-1] <= 'Z') {
				b.WriteByte('_')
			}
			c += 'a' - 'A'
		}
		b.WriteByte(c)
	}
	return b.String()
}

func ScriptSource(pkg, name string) string {
	return "package " + pkg + "\n\nscript " + name + " {\n}\n"
}

func CreateScript(ctx *context.Editor, dir, name string) (string, error) {
	if !ValidIdent(name) {
		return "", errs.New(errs.EditorDocument, name+" is not a valid script name")
	}
	rel := path.Join(dir, FileStem(name)+document.ScriptExt)
	if _, err := CreateFile(ctx, rel, []byte(ScriptSource(PackageOf(ctx, dir), name))); err != nil {
		return "", err
	}
	return rel, nil
}

func Delete(ctx *context.Editor, list []ids.AssetID) error {
	for _, id := range list {
		batch, err := ctx.Pipeline.Delete(id)
		ctx.ApplyBatch(batch)
		if err != nil {
			return err
		}
	}
	return nil
}

func DeleteDir(ctx *context.Editor, dir string) error {
	pl := ctx.Pipeline
	for _, a := range pl.Assets() {
		if !Under(a.Path, dir) {
			continue
		}
		batch, err := pl.Delete(a.ID)
		ctx.ApplyBatch(batch)
		if err != nil {
			return err
		}
	}
	var sub []string
	for _, d := range pl.Dirs() {
		if Under(d, dir) {
			sub = append(sub, d)
		}
	}
	sort.Slice(sub, func(i, j int) bool { return strings.Count(sub[i], "/") > strings.Count(sub[j], "/") })
	for _, d := range append(sub, dir) {
		if err := pl.RemoveDir(d); err != nil {
			return err
		}
	}
	return nil
}

func Duplicate(ctx *context.Editor, id ids.AssetID) (ids.AssetID, error) {
	dup, batch, err := ctx.Pipeline.Duplicate(id)
	ctx.ApplyBatch(batch)
	return dup, err
}

func withExt(rel, ext string) string {
	if path.Ext(rel) == ext {
		return rel
	}
	return rel + ext
}

func PromptSaveAs(ctx *context.Editor, d *document.SceneDocument) {
	if d == nil {
		return
	}
	id := d.ID
	ctx.Ask(context.Question{
		Title:   i18n.Tr("editor.assetops.save_scene_as", "Save scene as"),
		Fields:  []string{i18n.Tr("editor.assetops.path", "Path")},
		Values:  []string{d.Path},
		Buttons: []string{i18n.Tr("editor.assetops.save", "Save"), i18n.Tr("editor.common.cancel", "Cancel")},
		Submit: func(v []string) {
			rel := withExt(document.Clean(strings.TrimSpace(v[0])), document.SceneExt)
			if err := ctx.Docs.SaveAs(id, rel); err != nil {
				ctx.Notify(context.LevelError, err.Error())
				return
			}
			ctx.Notify(context.LevelInfo, i18n.Note("editor.assetops.saved", "Saved {path}", i18n.Str("path", rel)))
		},
	})
}

func PromptRenameScene(ctx *context.Editor, ops *Ops, d *document.SceneDocument) {
	if d == nil {
		return
	}
	if ops == nil {
		ops = &Ops{}
	}
	id, old, onDisk := d.ID, d.Path, d.OnDisk
	ctx.Ask(context.Question{
		Title:   i18n.Tr("editor.assetops.rename_scene", "Rename scene"),
		Fields:  []string{i18n.Tr("editor.assetops.name", "Name")},
		Values:  []string{Stem(old)},
		Buttons: []string{i18n.Tr("editor.assetops.rename", "Rename"), i18n.Tr("editor.common.cancel", "Cancel")},
		Submit: func(v []string) {
			name := strings.TrimSpace(v[0])
			if !ValidName(name) {
				ctx.Notify(context.LevelError, name+" is not a valid file name")
				return
			}
			dst := path.Join(path.Dir(old), name+document.SceneExt)
			if dst == old {
				return
			}
			if Exists(ctx, dst) {
				ctx.Notify(context.LevelError, dst+" already exists")
				return
			}
			asset, ok := ctx.Pipeline.Lookup(old)
			if !onDisk || !ok {
				if err := ctx.Docs.SaveAs(id, dst); err != nil {
					ctx.Notify(context.LevelError, err.Error())
				}
				return
			}
			if err := ops.Apply(ctx, []FileOp{{ID: asset, From: old, To: dst}}); err != nil {
				ctx.Notify(context.LevelError, err.Error())
			}
		},
	})
}
