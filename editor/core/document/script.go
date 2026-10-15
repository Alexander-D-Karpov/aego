package document

import (
	"encoding/hex"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"aego/core/errs"
	"aego/core/log"
	"aego/editor/core/code"
)

const ScriptExt = ".aes"

type ScriptDocument struct {
	ID         ID
	Path       string
	Code       *code.Doc
	ReadOnly   bool
	Note       string
	DiskAt     time.Time
	DiskSize   int64
	OnDisk     bool
	removedAck bool
}

func (d *ScriptDocument) Dirty() bool {
	return d.Code.Dirty()
}

func (d *ScriptDocument) Name() string {
	return strings.TrimSuffix(path.Base(d.Path), ScriptExt)
}

func (m *Manager) Scripts() []*ScriptDocument {
	return m.scripts
}

func (m *Manager) Script(rel string) *ScriptDocument {
	rel = Clean(rel)
	for _, d := range m.scripts {
		if d.Path == rel {
			return d
		}
	}
	return nil
}

func (m *Manager) statScript(d *ScriptDocument) {
	if st, err := os.Stat(m.Abs(d.Path)); err == nil {
		d.OnDisk = true
		d.DiskAt, d.DiskSize = st.ModTime(), st.Size()
	}
}

func (m *Manager) addScript(rel string, data []byte) *ScriptDocument {
	m.next++
	d := &ScriptDocument{ID: m.next, Path: rel, Code: code.New(data)}
	m.statScript(d)
	m.scripts = append(m.scripts, d)
	return d
}

func (m *Manager) OpenScript(rel string) (*ScriptDocument, error) {
	rel = Clean(rel)
	if d := m.Script(rel); d != nil {
		return d, nil
	}
	data, err := os.ReadFile(m.Abs(rel))
	if err != nil {
		return nil, errs.Wrap(err, errs.EditorDocument, "cannot read the script").With(log.F("path", rel))
	}
	return m.addScript(rel, data), nil
}

func (m *Manager) SaveScript(d *ScriptDocument) error {
	if err := WriteAtomic(m.Abs(d.Path), d.Code.Bytes()); err != nil {
		return errs.Wrap(err, errs.EditorDocument, "cannot save the script").With(log.F("path", d.Path))
	}
	d.Code.MarkSaved()
	m.statScript(d)
	d.removedAck = false
	os.Remove(m.scriptRecoveryFile(d.Path))
	return nil
}

func (m *Manager) CloseScript(d *ScriptDocument) {
	for i, s := range m.scripts {
		if s == d {
			m.scripts = append(m.scripts[:i], m.scripts[i+1:]...)
			break
		}
	}
	os.Remove(m.scriptRecoveryFile(d.Path))
}

func (m *Manager) ReloadScript(d *ScriptDocument) error {
	data, err := os.ReadFile(m.Abs(d.Path))
	if err != nil {
		return errs.Wrap(err, errs.EditorDocument, "cannot read the script").With(log.F("path", d.Path))
	}
	d.Code = code.New(data)
	m.statScript(d)
	d.removedAck = false
	os.Remove(m.scriptRecoveryFile(d.Path))
	return nil
}

func (m *Manager) KeepScript(d *ScriptDocument) {
	m.statScript(d)
}

func (m *Manager) CheckExternalScripts() ([]*ScriptDocument, []*ScriptDocument) {
	var reloaded, conflicts []*ScriptDocument
	for _, d := range m.scripts {
		if !d.OnDisk {
			continue
		}
		st, err := os.Stat(m.Abs(d.Path))
		if err != nil {
			if os.IsNotExist(err) && !d.removedAck {
				d.removedAck = true
				conflicts = append(conflicts, d)
			}
			continue
		}
		if st.ModTime().Equal(d.DiskAt) && st.Size() == d.DiskSize {
			continue
		}
		if !d.Dirty() {
			if err := m.ReloadScript(d); err == nil {
				reloaded = append(reloaded, d)
			}
			continue
		}
		d.DiskAt, d.DiskSize = st.ModTime(), st.Size()
		conflicts = append(conflicts, d)
	}
	return reloaded, conflicts
}

func (m *Manager) scriptRecoveryFile(rel string) string {
	return filepath.Join(m.recoveryDir(), hex.EncodeToString([]byte(rel))+ScriptExt)
}

func (m *Manager) writeScriptRecovery() (int, error) {
	n := 0
	var first error
	for _, d := range m.scripts {
		if !d.Dirty() {
			os.Remove(m.scriptRecoveryFile(d.Path))
			continue
		}
		if err := WriteAtomic(m.scriptRecoveryFile(d.Path), d.Code.Bytes()); err != nil {
			if first == nil {
				first = errs.Wrap(err, errs.EditorDocument, "cannot write the script recovery file")
			}
			continue
		}
		n++
	}
	return n, first
}

func (m *Manager) FindScriptRecovery() []Recovery {
	entries, err := os.ReadDir(m.recoveryDir())
	if err != nil {
		return nil
	}
	var out []Recovery
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ScriptExt) {
			continue
		}
		raw, err := hex.DecodeString(strings.TrimSuffix(name, ScriptExt))
		if err != nil {
			continue
		}
		rel := string(raw)
		if m.Script(rel) != nil {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		full := filepath.Join(m.recoveryDir(), name)
		if st, err := os.Stat(m.Abs(rel)); err == nil && !st.ModTime().Before(info.ModTime()) {
			os.Remove(full)
			continue
		}
		out = append(out, Recovery{Path: rel, File: full, Time: info.ModTime()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func (m *Manager) RestoreScriptRecovery(r Recovery) (*ScriptDocument, error) {
	if d := m.Script(r.Path); d != nil {
		return d, nil
	}
	data, err := os.ReadFile(r.File)
	if err != nil {
		return nil, errs.Wrap(err, errs.EditorDocument, "cannot read the script recovery file")
	}
	d := m.addScript(r.Path, data)
	disk, _ := os.ReadFile(m.Abs(r.Path))
	d.Code.SetSaved(disk)
	return d, nil
}
