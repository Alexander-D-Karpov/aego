package refactor

import (
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"

	"aego/asset/pipeline"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/project"
	"aego/project/rename"
)

const (
	ScriptExt     = ".aes"
	defaultModule = "game"
)

var packageDecl = regexp.MustCompile(`(?m)^[ \t]*package[ \t]+([A-Za-z_][A-Za-z0-9_]*)`)

type Op struct {
	ID   ids.AssetID
	From string
	To   string
}

func PackageSpan(src []byte) (int, int, bool) {
	m := packageDecl.FindSubmatchIndex(src)
	if m == nil {
		return 0, 0, false
	}
	return m[2], m[3], true
}

func SetPackage(src []byte, pkg string) []byte {
	a, b, ok := PackageSpan(src)
	if !ok {
		return append([]byte("package "+pkg+"\n\n"), src...)
	}
	out := append([]byte(nil), src[:a]...)
	out = append(out, pkg...)
	return append(out, src[b:]...)
}

func Module(p *project.Project) string {
	if p != nil && p.Module != "" {
		return p.Module
	}
	return defaultModule
}

func PackageOf(p *project.Project, dir string) string {
	if rel, ok := strings.CutPrefix(dir, "scripts/"); ok && rel != "" {
		return path.Base(rel)
	}
	m := Module(p)
	return m[strings.LastIndexByte(m, '/')+1:]
}

func Move(p *project.Project, pl *pipeline.Pipeline, id ids.AssetID, to string) (string, error) {
	from, ok := pl.PathOf(id)
	if !ok {
		return "", errs.New(errs.AssetNotFound, "asset is not in the project", log.F("asset", id.String()))
	}
	if err := pl.Move(id, to); err != nil {
		return "", err
	}
	if path.Ext(to) != ScriptExt {
		return "", nil
	}
	was, now := PackageOf(p, path.Dir(from)), PackageOf(p, path.Dir(to))
	if was == now {
		return "", nil
	}
	abs := pl.Abs(to)
	src, err := os.ReadFile(abs)
	if err != nil {
		return "", errs.Wrap(err, errs.VFSIO, "cannot read "+to)
	}
	if err := project.WriteAtomic(abs, SetPackage(src, now)); err != nil {
		return "", errs.Wrap(err, errs.VFSIO, "cannot update the package of "+to)
	}
	return path.Base(to) + " is now in package " + now + " (was " + was + "), fix the imports that used it", nil
}

func RenameOps(paths []string, list []ids.AssetID, rows []rename.Row) []Op {
	finals := map[string]bool{}
	sources := map[string]bool{}
	var ops []Op
	for i, rel := range paths {
		rw := rows[i]
		if !rw.Changed() {
			continue
		}
		to := path.Join(path.Dir(rel), rw.To+path.Ext(rel))
		finals[strings.ToLower(to)] = true
		sources[strings.ToLower(rel)] = true
		ops = append(ops, Op{ID: list[i], From: rel, To: to})
	}
	chain := false
	for to := range finals {
		if sources[to] {
			chain = true
			break
		}
	}
	if !chain {
		return ops
	}
	var temp, final []Op
	for i, op := range ops {
		tmp := path.Join(path.Dir(op.From), "__rename_"+strconv.Itoa(i)+"_"+path.Base(op.From))
		temp = append(temp, Op{ID: op.ID, From: op.From, To: tmp})
		final = append(final, Op{ID: op.ID, From: tmp, To: op.To})
	}
	return append(temp, final...)
}

func PlanRename(pl *pipeline.Pipeline, rule rename.Rule, list []ids.AssetID) ([]rename.Row, []Op, error) {
	var paths []string
	var keep []ids.AssetID
	var items []rename.Item
	for _, id := range list {
		rel, ok := pl.PathOf(id)
		if !ok {
			return nil, nil, errs.New(errs.AssetNotFound, "asset is not in the project", log.F("asset", id.String()))
		}
		ext := path.Ext(rel)
		paths = append(paths, rel)
		keep = append(keep, id)
		items = append(items, rename.Item{Name: strings.TrimSuffix(path.Base(rel), ext), Ext: ext, Parent: path.Dir(rel)})
	}
	rows, err := rename.Plan(rule, items, rename.ValidName, func(parent, name string) bool {
		_, err := os.Stat(pl.Abs(path.Join(parent, name)))
		return err == nil
	}, true)
	if err != nil {
		return nil, nil, err
	}
	if rename.Blocked(rows) {
		return rows, nil, errs.New(errs.VFSBadPath, "the new names conflict")
	}
	return rows, RenameOps(paths, keep, rows), nil
}

func ApplyOps(p *project.Project, pl *pipeline.Pipeline, ops []Op) ([]string, error) {
	var notes []string
	for i, op := range ops {
		note, err := Move(p, pl, op.ID, op.To)
		if err != nil {
			for j := i - 1; j >= 0; j-- {
				Move(p, pl, ops[j].ID, ops[j].From)
			}
			return nil, err
		}
		if note != "" {
			notes = append(notes, note)
		}
	}
	return notes, nil
}
