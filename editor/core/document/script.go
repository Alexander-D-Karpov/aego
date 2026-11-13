package document

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"aego/core/errs"
	"aego/core/log"
	"aego/editor/core/code"
)

const ScriptExt = ".aes"

type ScriptDocument struct {
	ID       ID
	Path     string
	Code     *code.Doc
	ReadOnly bool
	// Metadata has explicit Save/Save All. Undo and preview never become an
	// implicit source write through the code editor's autosave policy.
	ExplicitSave bool
	ValidateSave func([]byte) error
	SavedEdits   uint64
	Note         string
	DiskAt       time.Time
	DiskSize     int64
	OnDisk       bool
	// Conflict remains set until the user reloads or explicitly keeps this
	// buffer. Save and autosave cannot overwrite an unacknowledged disk change.
	Conflict    bool
	removedAck  bool
	diskHash    [32]byte
	diskTracked bool
	views       int
}

// RetainView keeps the shared text journal alive while a Code view is open,
// even if a material/asset view of the same source closes first.
func (d *ScriptDocument) RetainView() func() {
	d.views++
	live := true
	return func() {
		if live {
			d.views--
			live = false
		}
	}
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
	d.diskHash, d.diskTracked = sha256.Sum256(data), true
	d.Code.GLSL = shaderText(rel)
	m.statScript(d)
	m.scripts = append(m.scripts, d)
	return d
}

func (m *Manager) OpenScript(rel string) (*ScriptDocument, error) {
	rel = Clean(rel)
	if !validProjectPath(rel) {
		return nil, errs.New(errs.EditorDocument, "source path must stay inside the project")
	}
	if d := m.Script(rel); d != nil {
		return d, nil
	}
	data, err := m.ReadSource(rel)
	if err != nil {
		return nil, errs.Wrap(err, errs.EditorDocument, "cannot read the script").With(log.F("path", rel))
	}
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return nil, errs.New(errs.EditorDocument, "binary source cannot be opened in the text editor").With(log.F("path", rel))
	}
	return m.addScript(rel, data), nil
}

func volatileSaveError() error {
	return errs.New(errs.EditorDocument, "in-memory structured changes cannot be saved as text")
}

func (m *Manager) volatileScript() *ScriptDocument {
	for _, d := range m.scripts {
		if d.Code.VolatileDirty() {
			return d
		}
	}
	for _, a := range m.assets {
		for _, d := range []*ScriptDocument{a.Source, a.Metadata} {
			if d != nil && d.Code.VolatileDirty() {
				return d
			}
		}
	}
	return nil
}

// CheckVolatileSave refuses Save All while an open structured document has
// unsupported in-memory edits. It does not make multiple file saves atomic.
// The manager and its callbacks use the owner thread.
func (m *Manager) CheckVolatileSave() error {
	if d := m.volatileScript(); d != nil {
		return volatileSaveError()
	}
	return nil
}

func (m *Manager) SaveScript(d *ScriptDocument) error {
	if d == nil || d.ReadOnly {
		return errs.New(errs.EditorDocument, "source document is read-only")
	}
	if d.Code.VolatileDirty() {
		return errs.New(errs.EditorDocument, "in-memory structured changes cannot be saved as text")
	}
	if m.relatedVolatile(d) {
		return volatileSaveError()
	}
	if d.ValidateSave != nil {
		if err := d.ValidateSave(d.Code.Bytes()); err != nil {
			return err
		}
	}
	if err := m.writeSource(func() error {
		if d.Code.VolatileDirty() || m.relatedVolatile(d) {
			return volatileSaveError()
		}
		if d.Conflict || !m.scriptMatchesDisk(d) {
			d.Conflict = true
			return errs.New(errs.EditorDocument, "source changed on disk; reload or explicitly keep your changes before saving").With(log.F("path", d.Path))
		}
		if err := WriteAtomic(m.Abs(d.Path), d.Code.Bytes()); err != nil {
			return errs.Wrap(err, errs.EditorDocument, "cannot save the script").With(log.F("path", d.Path))
		}
		return nil
	}); err != nil {
		return err
	}
	// A host callback may have introduced structured work after performing the
	// write. Do not acknowledge or clear recovery for that newer state; the
	// earlier filesystem write itself cannot be rolled back here.
	if d.Code.VolatileDirty() || m.relatedVolatile(d) {
		return volatileSaveError()
	}
	d.Code.MarkSaved()
	d.SavedEdits++
	d.diskHash, d.diskTracked = sha256.Sum256(d.Code.Bytes()), true
	m.statScript(d)
	d.removedAck = false
	m.clearScriptRecovery(d.Path)
	m.emit(Event{Kind: EvSourceSaved, Doc: d.ID, Path: d.Path})
	return nil
}

func (m *Manager) CloseScript(d *ScriptDocument) {
	if m.scriptOwned(d) || d.views > 0 || d.Code.VolatileDirty() {
		return
	}
	for i, s := range m.scripts {
		if s == d {
			m.scripts = append(m.scripts[:i], m.scripts[i+1:]...)
			break
		}
	}
	m.clearScriptRecovery(d.Path)
}

// DiscardScript is explicit and shared by all views. Hiding a Code tab alone
// cannot discard its journal now that an asset view can retain the same model.
func (m *Manager) DiscardScript(d *ScriptDocument) error {
	if err := m.reloadScript(d, true); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		// Discarding a removed file does not recreate it or keep hidden edits.
		d.Code = code.New(nil)
		d.Code.GLSL = shaderText(d.Path)
		d.Conflict, d.OnDisk, d.removedAck = false, false, true
		d.diskHash, d.diskTracked = sha256.Sum256(nil), true
		m.clearScriptRecovery(d.Path)
		m.emit(Event{Kind: EvScriptReplaced, Doc: d.ID, Path: d.Path})
	}
	return nil
}

func (m *Manager) ReloadScript(d *ScriptDocument) error {
	return m.reloadScript(d, false)
}

func (m *Manager) reloadScript(d *ScriptDocument, discardVolatile bool) error {
	if !discardVolatile && d.Code.VolatileDirty() {
		return errs.New(errs.EditorDocument, "discard in-memory structured changes explicitly before reloading")
	}
	data, err := os.ReadFile(m.Abs(d.Path))
	if err != nil {
		return errs.Wrap(err, errs.EditorDocument, "cannot read the script").With(log.F("path", d.Path))
	}
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return errs.New(errs.EditorDocument, "binary source cannot replace a text document")
	}
	d.Code = code.New(data)
	d.diskHash, d.diskTracked, d.Conflict = sha256.Sum256(data), true, false
	d.Code.GLSL = shaderText(d.Path)
	m.statScript(d)
	d.removedAck = false
	m.clearScriptRecovery(d.Path)
	m.emit(Event{Kind: EvScriptReplaced, Doc: d.ID, Path: d.Path})
	return nil
}

// AcceptSavedScript adopts a source transaction already committed to disk. It
// retains the document history and any recovery file, and refuses a different
// buffer or postimage. As with other filesystem guards, this is not atomic CAS.
func (m *Manager) AcceptSavedScript(d *ScriptDocument, before, after []byte) error {
	if d == nil || m.Script(d.Path) != d || d.ReadOnly || d.Dirty() || d.Conflict ||
		m.relatedVolatile(d) || !d.OnDisk || !d.diskTracked || sha256.Sum256(before) != d.diskHash ||
		!bytes.Equal(d.Code.Bytes(), normalizeScriptText(before)) {
		return errs.New(errs.EditorDocument, "source document changed before accepting the saved transaction")
	}
	if !utf8.Valid(after) || bytes.IndexByte(after, 0) >= 0 {
		return errs.New(errs.EditorDocument, "saved source cannot replace a text document")
	}
	f, err := m.OpenSource(d.Path)
	if err != nil {
		return err
	}
	data, readErr := io.ReadAll(io.LimitReader(f, int64(len(after))+1))
	if err := errors.Join(readErr, f.Close()); err != nil {
		return err
	}
	if !bytes.Equal(data, after) {
		return errs.New(errs.EditorDocument, "saved source no longer matches the accepted transaction")
	}
	if d.Code.VolatileDirty() || m.relatedVolatile(d) {
		return volatileSaveError()
	}
	d.Code.ReplaceSnapshot(after, false)
	d.Code.MarkSaved()
	d.SavedEdits++
	d.diskHash, d.diskTracked = sha256.Sum256(after), true
	d.removedAck = false
	m.statScript(d)
	return nil
}

// The text editor normalizes line endings but the guarded disk checkpoint must
// continue to describe the exact original file bytes.
func normalizeScriptText(b []byte) []byte {
	return bytes.ReplaceAll(bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")), []byte("\r"), []byte("\n"))
}

func (m *Manager) KeepScript(d *ScriptDocument) {
	data, err := os.ReadFile(m.Abs(d.Path))
	if err != nil && !os.IsNotExist(err) {
		return
	}
	d.diskHash, d.diskTracked = sha256.Sum256(data), true
	d.Conflict = false
	d.OnDisk = err == nil
	if d.OnDisk {
		m.statScript(d)
	}
}

func (m *Manager) scriptMatchesDisk(d *ScriptDocument) bool {
	if !d.diskTracked {
		return true
	}
	data, err := os.ReadFile(m.Abs(d.Path))
	if os.IsNotExist(err) {
		return !d.OnDisk
	}
	if err != nil || !d.OnDisk {
		return false
	}
	return sha256.Sum256(data) == d.diskHash
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
				d.Conflict = true
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
		d.Conflict = true
		conflicts = append(conflicts, d)
	}
	return reloaded, conflicts
}

func (m *Manager) scriptRecoveryFile(rel string) string {
	return filepath.Join(m.recoveryDir(), hex.EncodeToString([]byte(rel))+ScriptExt)
}

func (m *Manager) clearScriptRecovery(rel string) {
	if err := m.discardOwnedRecovery(m.scriptRecoveryFile(rel)); err != nil {
		m.log.Warn("cannot clear changed script recovery file", log.F("path", rel), log.F("err", err.Error()))
	}
}

func (m *Manager) writeScriptRecovery() (int, error) {
	n := 0
	var first error
	for _, d := range m.scripts {
		if d.Code.VolatileDirty() {
			if first == nil {
				first = errs.New(errs.EditorDocument, "in-memory structured changes have no recovery format")
			}
			continue
		}
		if !d.Dirty() {
			m.clearScriptRecovery(d.Path)
			continue
		}
		name, data := m.scriptRecoveryFile(d.Path), d.Code.Bytes()
		if err := m.writeRecoveryFile(name, data); err != nil {
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
			// Do not delete an old copy merely because the source is newer.
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
	d.diskHash, d.diskTracked = sha256.Sum256(disk), true
	if filepath.Clean(r.File) == filepath.Clean(m.scriptRecoveryFile(r.Path)) {
		m.trackRecovery(r.File, data)
	}
	return d, nil
}

func shaderText(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".aeshader", ".glsl", ".vert", ".frag":
		return true
	}
	return false
}
