package refactor

import (
	"bytes"
	"os"
	"path/filepath"
	"time"

	"aego/asset/pipeline"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/project"
	"aego/project/refs"
)

const RecoveryDir = ".aego/recovery"

type Edit struct {
	ID     ids.AssetID
	Path   string
	Before []byte
	After  []byte
}

type Replacement struct {
	From    ids.AssetID
	To      ids.AssetID
	Project bool
	Files   []string
}

func BackupDir(now time.Time) string {
	return filepath.ToSlash(filepath.Join(RecoveryDir, "refs-"+now.Format("20060102-150405")))
}

func CheckReplace(src refs.Source, from, to ids.AssetID) error {
	if from == to {
		return errs.New(errs.EditorCommand, "an asset cannot replace itself")
	}
	if to.IsZero() {
		return nil
	}
	fe, fok := src.Entry(from)
	te, tok := src.Entry(to)
	if !tok {
		return errs.New(errs.EditorCommand, "the replacement is not an imported asset")
	}
	if fok && fe.Kind != te.Kind {
		return errs.New(errs.EditorCommand, "the replacement is a "+string(te.Kind)+", the references expect a "+string(fe.Kind))
	}
	return nil
}

func PlanReplace(p *project.Project, src refs.Source, from, to ids.AssetID, skip func(rel string) bool) (*Replacement, error) {
	if err := CheckReplace(src, from, to); err != nil {
		return nil, err
	}
	r := &Replacement{From: from, To: to}
	for _, ref := range refs.Referrers(p, src, from) {
		switch {
		case ref.Project:
			r.Project = true
		case skip != nil && skip(ref.Path):
		default:
			r.Files = append(r.Files, ref.Path)
		}
	}
	return r, nil
}

func (r *Replacement) Edits(abs func(string) string) ([]Edit, error) {
	old, now := refs.Token(r.From), refs.Token(r.To)
	var out []Edit
	for _, rel := range r.Files {
		before, err := os.ReadFile(abs(rel))
		if err != nil {
			return nil, errs.Wrap(err, errs.VFSIO, "cannot read "+rel)
		}
		if !bytes.Contains(before, old) {
			continue
		}
		out = append(out, Edit{Path: rel, Before: before, After: bytes.ReplaceAll(before, old, now)})
	}
	return out, nil
}

func (r *Replacement) Write(root string, abs func(string) string, backup string) ([]string, error) {
	edits, err := r.Edits(abs)
	if err != nil {
		return nil, err
	}
	return WriteEdits(root, abs, backup, edits)
}

func (r *Replacement) ApplyProject(p *project.Project) error {
	if !r.Project || r.To.IsZero() || p == nil {
		return nil
	}
	return project.ChangeMainScene(p, p.MainScene, r.To)
}

func WriteEdits(root string, abs func(string) string, backup string, edits []Edit) ([]string, error) {
	var done []string
	dir := filepath.Join(root, filepath.FromSlash(backup))
	for _, e := range edits {
		if backup != "" {
			bak := filepath.Join(dir, filepath.FromSlash(e.Path))
			if err := os.MkdirAll(filepath.Dir(bak), 0o755); err != nil {
				return done, errs.Wrap(err, errs.VFSIO, "cannot create the backup folder").With(log.F("backup", backup))
			}
			if err := os.WriteFile(bak, e.Before, 0o644); err != nil {
				return done, errs.Wrap(err, errs.VFSIO, "cannot back up "+e.Path).With(log.F("backup", backup))
			}
		}
		if err := project.WriteAtomic(abs(e.Path), e.After); err != nil {
			return done, errs.Wrap(err, errs.VFSIO, "cannot write "+e.Path).With(log.F("backup", backup))
		}
		done = append(done, e.Path)
	}
	return done, nil
}

func WriteAll(abs func(string) string, edits []Edit) error {
	for i, e := range edits {
		if err := project.WriteAtomic(abs(e.Path), e.After); err != nil {
			for _, w := range edits[:i] {
				project.WriteAtomic(abs(w.Path), w.Before)
			}
			return errs.Wrap(err, errs.VFSIO, "cannot write "+e.Path)
		}
	}
	return nil
}

func Reimport(pl *pipeline.Pipeline, rels []string) (*pipeline.Batch, error) {
	var list []ids.AssetID
	for _, rel := range rels {
		if id, ok := pl.Lookup(rel); ok {
			list = append(list, id)
		}
	}
	if len(list) == 0 {
		return nil, nil
	}
	return pl.Reimport(list...)
}
