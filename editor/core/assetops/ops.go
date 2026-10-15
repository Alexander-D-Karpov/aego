package assetops

import (
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"aego/core/errs"
	"aego/core/ids"
	"aego/editor/core/context"
	"aego/editor/core/document"
)

const (
	PrefabExt = ".aeprefab"
	opsLimit  = 64
)

var packageDecl = regexp.MustCompile(`(?m)^[ \t]*package[ \t]+([A-Za-z_][A-Za-z0-9_]*)`)

type FileOp struct {
	Dir  bool
	ID   ids.AssetID
	From string
	To   string
}

type Ops struct {
	undo [][]FileOp
	redo [][]FileOp
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
		o.undo = append(o.undo, done)
		if over := len(o.undo) - opsLimit; over > 0 {
			o.undo = append(o.undo[:0], o.undo[over:]...)
		}
		o.redo = o.redo[:0]
	}
	return err
}

func (o *Ops) Undo(ctx *context.Editor) error {
	n := len(o.undo)
	if n == 0 {
		return nil
	}
	step := o.undo[n-1]
	o.undo = o.undo[:n-1]
	if _, err := runOps(ctx, step, true); err != nil {
		o.undo, o.redo = o.undo[:0], o.redo[:0]
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
	if _, err := runOps(ctx, step, false); err != nil {
		o.undo, o.redo = o.undo[:0], o.redo[:0]
		return err
	}
	o.undo = append(o.undo, step)
	return nil
}

func runOps(ctx *context.Editor, ops []FileOp, back bool) ([]FileOp, error) {
	done := make([]FileOp, 0, len(ops))
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
	from, to := f.From, f.To
	if back {
		from, to = to, from
	}
	if f.Dir {
		return moveDir(ctx, from, to)
	}
	return MoveFile(ctx, f.ID, from, to)
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
		ctx.Notify(context.LevelError, "cannot update the package of "+to+": "+err.Error())
		return
	}
	ctx.Notify(context.LevelWarn, path.Base(to)+" is now in package "+now+" (was "+was+"), fix the imports that used it")
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
	if m := packageDecl.FindSubmatchIndex(c.Bytes()); m != nil {
		c.ReplaceRanges([][2]int{{m[2], m[3]}}, [][]byte{[]byte(pkg)})
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
	return s != "" && s != "." && s != ".." && !strings.HasPrefix(s, ".") && !strings.ContainsAny(s, "/\\")
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
	if rel, ok := ScriptsRel(dir); ok && rel != "" {
		return path.Base(rel)
	}
	m := moduleOf(ctx)
	return m[strings.LastIndexByte(m, '/')+1:]
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
		Title:   "Save scene as",
		Fields:  []string{"Path"},
		Values:  []string{d.Path},
		Buttons: []string{"Save", "Cancel"},
		Submit: func(v []string) {
			rel := withExt(document.Clean(strings.TrimSpace(v[0])), document.SceneExt)
			if err := ctx.Docs.SaveAs(id, rel); err != nil {
				ctx.Notify(context.LevelError, err.Error())
				return
			}
			ctx.Notify(context.LevelInfo, "Saved "+rel)
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
		Title:   "Rename scene",
		Fields:  []string{"Name"},
		Values:  []string{Stem(old)},
		Buttons: []string{"Rename", "Cancel"},
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
