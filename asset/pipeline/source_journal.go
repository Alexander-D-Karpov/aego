package pipeline

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"aego/core/ids"
	"aego/project"
)

const (
	sourceJournalParent = ".aego/source-transactions"
	sourceJournalDir    = sourceJournalParent + "/active"
	sourceJournalFile   = sourceJournalDir + "/journal.jsonl"
	maxJournalBytes     = 8 << 20
	maxJournalEdits     = 4096
)

// sourceVersion identifies bytes that this transaction was allowed to publish.
// Missing and an existing zero-byte file are intentionally different.
type sourceVersion struct {
	Exists bool   `json:"exists"`
	SHA256 string `json:"sha256,omitempty"`
}

func sourceVersionOf(data []byte) sourceVersion {
	if data == nil {
		return sourceVersion{}
	}
	sum := sha256.Sum256(data)
	return sourceVersion{Exists: true, SHA256: hex.EncodeToString(sum[:])}
}

func (v sourceVersion) valid() bool {
	if !v.Exists {
		return v.SHA256 == ""
	}
	if len(v.SHA256) != sha256.Size*2 || strings.ToLower(v.SHA256) != v.SHA256 {
		return false
	}
	_, err := hex.DecodeString(v.SHA256)
	return err == nil
}

type sourceJournalEntry struct {
	Path   string        `json:"path"`
	Before sourceVersion `json:"before"`
	After  sourceVersion `json:"after"`
	Backup string        `json:"backup,omitempty"`
}

type sourceJournalRecord struct {
	Kind    string               `json:"kind"`
	Version int                  `json:"version,omitempty"`
	ID      string               `json:"id,omitempty"`
	Entries []sourceJournalEntry `json:"entries,omitempty"`
	Path    string               `json:"path,omitempty"`
	Value   sourceVersion        `json:"value,omitempty"`
}

type sourceJournal struct {
	root       *os.Root
	dirInfo    os.FileInfo
	id         string
	entries    []sourceJournalEntry
	allowed    map[string][]sourceVersion
	state      string
	observed   []byte
	checkpoint func(stage, rel string)
}

// syncSourceDirs makes a rename durable on platforms that support directory
// fsync. Windows does not expose it through os.File.Sync; process-crash
// recovery is still available there, but power-loss durability is not claimed.
func syncSourceDirs(root *os.Root, dir string) error {
	if runtime.GOOS == "windows" || runtime.GOOS == "js" || runtime.GOOS == "wasip1" {
		return nil
	}
	for {
		f, err := root.Open(dir)
		if err != nil {
			return err
		}
		err = errors.Join(f.Sync(), f.Close())
		if err != nil {
			return err
		}
		if dir == "." {
			return nil
		}
		dir = path.Dir(dir)
	}
}

func journalDirectory(root *os.Root, dir string) error {
	info, err := root.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		if err := root.Mkdir(dir, 0o700); err != nil {
			return err
		}
		return syncSourceDirs(root, path.Dir(dir))
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("source recovery directory is not a real directory: %s", dir)
	}
	return nil
}

// sourceJournalPresent is intentionally only a presence check. An incomplete
// or malformed journal blocks normal imports just like a complete one; it is
// never interpreted as permission to alter or discard user files.
func sourceJournalPresent(root string) (bool, error) {
	_, err := os.Lstat(filepath.Join(root, filepath.FromSlash(sourceJournalDir)))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func sourceRecoveryRequired() error {
	return errors.New("a pending or interrupted source transaction needs attention; close other project processes and run 'aego source-recovery status' for this project")
}

// A journal belongs to this pipeline only while ApplySources holds mu. Once
// that call returns, leave pending recovery untouched and block later pipeline
// writes, including watcher imports and sidecar repair, until it is resolved.
func (p *Pipeline) requireNoPendingSourceRecovery() error {
	if p.sourceJournal != nil {
		return nil
	}
	pending, err := sourceJournalPresent(p.root)
	if err != nil {
		return fmt.Errorf("cannot inspect source recovery: %w", err)
	}
	if pending {
		return sourceRecoveryRequired()
	}
	return nil
}

func (p *Pipeline) requireWritableSources() error {
	if p.shared {
		return errors.New("another process holds the asset index; change source files there")
	}
	return p.requireNoPendingSourceRecovery()
}

// CheckSourceRecovery waits for this pipeline's active source transaction and
// rejects a pending journal without modifying it. It is a point-in-time check,
// not a lock across subsequent reads or a guard against external processes.
// Callers collecting multiple inputs must also validate their exact preimages.
func (p *Pipeline) CheckSourceRecovery() error {
	if p == nil {
		return errors.New("source recovery check needs an active pipeline")
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.requireNoPendingSourceRecovery()
}

// GuardSourceWrite serializes an ordinary editor save with this pipeline's
// source transactions and refuses the save while a recovery journal is
// pending. The callback must only perform its file preimage check and write;
// it must not call back into Pipeline. Notifications and imports run after the
// callback returns. Other/older processes and external editors do not share
// this mutex, so this is not an atomic CAS or a universal filesystem lock.
func (p *Pipeline) GuardSourceWrite(write func() error) error {
	if p == nil || write == nil {
		return errors.New("source write needs an active pipeline and callback")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.requireNoPendingSourceRecovery(); err != nil {
		return err
	}
	return write()
}

func startSourceJournal(root *os.Root, edits []SourceEdit, back bool) (*sourceJournal, error) {
	if len(edits) > maxJournalEdits {
		return nil, fmt.Errorf("source transaction exceeds %d files", maxJournalEdits)
	}
	for _, dir := range []string{".aego", sourceJournalParent} {
		if err := journalDirectory(root, dir); err != nil {
			return nil, fmt.Errorf("cannot prepare source recovery: %w", err)
		}
	}
	if err := root.Mkdir(sourceJournalDir, 0o700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, sourceRecoveryRequired()
		}
		return nil, fmt.Errorf("cannot create source recovery: %w", err)
	}
	if err := syncSourceDirs(root, sourceJournalParent); err != nil {
		return nil, fmt.Errorf("cannot persist source recovery directory: %w", err)
	}
	info, err := root.Lstat(sourceJournalDir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("source recovery directory changed during preparation")
	}
	j := &sourceJournal{root: root, dirInfo: info, id: ids.NewAssetID().String(), allowed: make(map[string][]sourceVersion), state: "prepared"}
	for i, e := range edits {
		if err := j.checkDirectory(); err != nil {
			return nil, err
		}
		before, after := e.Before, e.After
		if back {
			before, after = after, before
		}
		entry := sourceJournalEntry{Path: e.Path, Before: sourceVersionOf(before), After: sourceVersionOf(after)}
		if before != nil {
			entry.Backup = fmt.Sprintf("before-%04d", i)
			f, err := root.OpenFile(path.Join(sourceJournalDir, entry.Backup), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if err != nil {
				return nil, fmt.Errorf("cannot create recovery backup for %s: %w", e.Path, err)
			}
			_, writeErr := f.Write(before)
			err = errors.Join(writeErr, f.Sync(), f.Close())
			if err != nil {
				return nil, fmt.Errorf("cannot persist recovery backup for %s: %w", e.Path, err)
			}
		}
		j.entries = append(j.entries, entry)
		j.allowed[e.Path] = []sourceVersion{entry.Before, entry.After}
	}
	header := sourceJournalRecord{Kind: "prepared", Version: 1, ID: j.id, Entries: j.entries}
	data, err := json.Marshal(header)
	if err != nil || len(data)+1 > maxJournalBytes {
		return nil, errors.New("source recovery journal is too large")
	}
	if err := j.checkDirectory(); err != nil {
		return nil, err
	}
	f, err := root.OpenFile(sourceJournalFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("cannot create source recovery journal: %w", err)
	}
	data = append(data, '\n')
	_, writeErr := f.Write(data)
	err = errors.Join(writeErr, f.Sync(), f.Close())
	if err == nil {
		err = syncSourceDirs(root, sourceJournalDir)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot persist source recovery journal: %w", err)
	}
	j.observed = bytes.Clone(data)
	return j, nil
}

// The directory identity is an additional best-effort guard. A replacement
// between this check and a filesystem operation is still not an atomic CAS.
func (j *sourceJournal) checkDirectory() error {
	info, err := j.root.Lstat(sourceJournalDir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !os.SameFile(j.dirInfo, info) {
		return errors.New("source recovery directory changed; refusing to touch its files")
	}
	return nil
}

func (j *sourceJournal) append(record sourceJournalRecord) error {
	if err := j.checkDirectory(); err != nil {
		return err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	current, err := readJournalFile(j.root, sourceJournalFile, maxJournalBytes)
	if err != nil || !bytes.Equal(current, j.observed) {
		return errors.New("source recovery journal changed before its update")
	}
	info, err := j.root.Lstat(sourceJournalFile)
	if err != nil || !info.Mode().IsRegular() || info.Size()+int64(len(data)+1) > maxJournalBytes {
		return errors.New("source recovery journal changed or is too large")
	}
	f, err := j.root.OpenFile(sourceJournalFile, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return err
	}
	opened, statErr := f.Stat()
	if statErr != nil || !os.SameFile(info, opened) || info.Size() != opened.Size() {
		f.Close()
		return errors.New("source recovery journal changed before its update")
	}
	data = append(data, '\n')
	_, writeErr := f.Write(data)
	if err := errors.Join(writeErr, f.Sync(), f.Close()); err != nil {
		return err
	}
	j.observed = append(j.observed, data...)
	return nil
}

// authorize persists the exact metadata candidate before the resolver writes it.
func (j *sourceJournal) authorize(rel string, data []byte) error {
	if err := j.checkDirectory(); err != nil {
		return err
	}
	versions, ok := j.allowed[rel]
	if !ok {
		return fmt.Errorf("%s has no source recovery guard", rel)
	}
	v := sourceVersionOf(data)
	if slices.Contains(versions, v) {
		return nil
	}
	if err := j.append(sourceJournalRecord{Kind: "allow", Path: rel, Value: v}); err != nil {
		return fmt.Errorf("cannot protect resolved metadata %s: %w", rel, err)
	}
	j.allowed[rel] = append(versions, v)
	return nil
}

// finish records the resolved state before deleting any backups. A crash during
// cleanup therefore cannot turn a committed or restored transaction into an
// apparently incomplete one whose missing backup might be applied blindly.
func (j *sourceJournal) finish(state string) error {
	if state != "committed" && state != "restored" {
		return errors.New("invalid source recovery state")
	}
	if err := j.append(sourceJournalRecord{Kind: state}); err != nil {
		return err
	}
	j.state = state
	if j.checkpoint != nil {
		j.checkpoint(state, "")
	}
	return j.cleanup()
}

func readJournalFile(root *os.Root, name string, limit int64) ([]byte, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("invalid source recovery file: %s", name)
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	opened, statErr := f.Stat()
	if statErr != nil || !os.SameFile(info, opened) {
		f.Close()
		return nil, fmt.Errorf("source recovery file changed: %s", name)
	}
	b, readErr := io.ReadAll(io.LimitReader(f, limit+1))
	after, afterErr := f.Stat()
	closeErr := f.Close()
	if err := errors.Join(readErr, afterErr, closeErr); err != nil {
		return nil, err
	}
	if int64(len(b)) > limit || !os.SameFile(info, after) || info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) {
		return nil, fmt.Errorf("source recovery file changed while reading: %s", name)
	}
	current, err := root.Lstat(name)
	if err != nil || !os.SameFile(after, current) || current.Size() != after.Size() || !current.ModTime().Equal(after.ModTime()) {
		return nil, fmt.Errorf("source recovery file was replaced while reading: %s", name)
	}
	if b == nil {
		b = []byte{}
	}
	return b, nil
}

func decodeJournalRecord(line []byte) (sourceJournalRecord, error) {
	var r sourceJournalRecord
	d := json.NewDecoder(bytes.NewReader(line))
	d.DisallowUnknownFields()
	if err := d.Decode(&r); err != nil {
		return r, err
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return r, errors.New("extra data in source recovery record")
	}
	return r, nil
}

func loadSourceJournal(root *os.Root, p *Pipeline) (*sourceJournal, error) {
	var dirInfo os.FileInfo
	for _, dir := range []string{".aego", sourceJournalParent, sourceJournalDir} {
		info, err := root.Lstat(dir)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("source recovery directory is missing or unsafe: %s", dir)
		}
		if dir == sourceJournalDir {
			dirInfo = info
		}
	}
	data, err := readJournalFile(root, sourceJournalFile, maxJournalBytes)
	if err != nil {
		return nil, fmt.Errorf("source recovery journal cannot be read; preserve and inspect %s before manual resolution: %w", sourceJournalDir, err)
	}
	// A partial last event may be a crash remnant or an external change. Do not
	// truncate or discard bytes whose ownership cannot be established.
	last := bytes.LastIndexByte(data, '\n')
	if last < 0 || last != len(data)-1 {
		return nil, fmt.Errorf("source recovery journal is incomplete; preserve and inspect %s before manual resolution", sourceJournalDir)
	}
	lines := bytes.Split(data[:last], []byte{'\n'})
	header, err := decodeJournalRecord(lines[0])
	if err != nil || header.Kind != "prepared" || header.Version != 1 || len(header.ID) != 32 || len(header.Entries) > maxJournalEdits {
		return nil, errors.New("invalid source recovery header")
	}
	if _, err := ids.ParseAssetID(header.ID); err != nil {
		return nil, errors.New("invalid source recovery ID")
	}
	j := &sourceJournal{root: root, dirInfo: dirInfo, id: header.ID, entries: header.Entries, allowed: make(map[string][]sourceVersion), state: "prepared", observed: bytes.Clone(data)}
	if err := j.checkDirectory(); err != nil {
		return nil, err
	}
	for i, e := range header.Entries {
		if !fs.ValidPath(e.Path) || !filepath.IsLocal(e.Path) || strings.ContainsAny(e.Path, "\\:#") || !p.inRoots(e.Path) || !e.Before.valid() || !e.After.valid() || j.allowed[e.Path] != nil {
			return nil, fmt.Errorf("invalid source recovery entry: %s", e.Path)
		}
		for _, part := range strings.Split(e.Path, "/") {
			if strings.HasPrefix(part, ".") {
				return nil, fmt.Errorf("hidden source recovery path: %s", e.Path)
			}
		}
		wantBackup := ""
		if e.Before.Exists {
			wantBackup = fmt.Sprintf("before-%04d", i)
		}
		if e.Backup != wantBackup {
			return nil, fmt.Errorf("invalid recovery backup for %s", e.Path)
		}
		j.allowed[e.Path] = []sourceVersion{e.Before, e.After}
	}
	for _, line := range lines[1:] {
		r, err := decodeJournalRecord(line)
		if err != nil || j.state != "prepared" {
			return nil, errors.New("invalid source recovery event")
		}
		switch r.Kind {
		case "allow":
			versions, ok := j.allowed[r.Path]
			if !ok || !r.Value.valid() || !strings.HasSuffix(r.Path, MetaSuffix) {
				return nil, errors.New("invalid source recovery metadata event")
			}
			j.allowed[r.Path] = append(versions, r.Value)
		case "committed", "restored":
			j.state = r.Kind
		default:
			return nil, errors.New("unknown source recovery event")
		}
	}
	return j, nil
}

func (j *sourceJournal) cleanup() error {
	if j.state != "committed" && j.state != "restored" {
		return errors.New("cannot clean unresolved source recovery")
	}
	if err := j.checkDirectory(); err != nil {
		return err
	}
	data, err := readJournalFile(j.root, sourceJournalFile, maxJournalBytes)
	if err != nil || !bytes.Equal(data, j.observed) {
		return errors.New("source recovery journal changed; refusing cleanup")
	}
	dir, err := j.root.Open(sourceJournalDir)
	if err != nil {
		return err
	}
	names, readErr := dir.Readdirnames(-1)
	err = errors.Join(readErr, dir.Close())
	if err != nil {
		return err
	}
	known := map[string]bool{path.Base(sourceJournalFile): true}
	for _, e := range j.entries {
		if e.Backup == "" {
			continue
		}
		known[e.Backup] = true
		data, err := readJournalFile(j.root, path.Join(sourceJournalDir, e.Backup), recoverySourceLimit(e.Path))
		if errors.Is(err, os.ErrNotExist) {
			continue // A previous cleanup may have stopped after resolving.
		}
		if err != nil || sourceVersionOf(data) != e.Before {
			return fmt.Errorf("recovery backup changed; refusing cleanup: %s", e.Backup)
		}
	}
	for _, name := range names {
		if !known[name] {
			return fmt.Errorf("unexpected recovery file; refusing cleanup: %s", name)
		}
	}
	for _, e := range j.entries {
		if e.Backup != "" {
			if err := j.checkDirectory(); err != nil {
				return err
			}
			if err := j.root.Remove(path.Join(sourceJournalDir, e.Backup)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	if err := j.checkDirectory(); err != nil {
		return err
	}
	if err := j.root.Remove(sourceJournalFile); err != nil {
		return err
	}
	if err := syncSourceDirs(j.root, sourceJournalDir); err != nil {
		return err
	}
	if err := j.root.Remove(sourceJournalDir); err != nil {
		return err
	}
	return syncSourceDirs(j.root, sourceJournalParent)
}

// SourceRecovery describes a pending transaction without changing its files.
type SourceRecovery struct {
	ID    string
	State string
	Paths []string
}

// SourceRecoveryStatus inspects a pending journal. It never performs recovery.
func SourceRecoveryStatus(prj *project.Project) (*SourceRecovery, error) {
	if prj == nil {
		return nil, errors.New("source recovery needs a project")
	}
	present, err := sourceJournalPresent(prj.Root)
	if err != nil || !present {
		return nil, err
	}
	root, err := os.OpenRoot(prj.Root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	j, err := loadSourceJournal(root, newPipeline(prj.Root, nil, nil))
	if err != nil {
		return nil, err
	}
	out := &SourceRecovery{ID: j.id, State: j.state}
	for _, e := range j.entries {
		out.Paths = append(out.Paths, e.Path)
	}
	return out, nil
}

// RestoreSourceRecovery explicitly restores the old source generation, or
// cleans a previously completed recovery. It refuses external changes and
// never starts while another pipeline owns the project's index lock.
func RestoreSourceRecovery(prj *project.Project) error {
	return restoreSourceRecovery(prj, nil)
}

// checkpoint is an internal per-call test seam; ordinary recovery always
// passes nil and still requires the caller's explicit action.
func restoreSourceRecovery(prj *project.Project, checkpoint func(stage, rel string)) error {
	if prj == nil {
		return errors.New("source recovery needs a project")
	}
	unlock, owner := acquireIndexLock(prj.CacheDir())
	if !owner {
		return errors.New("close the editor and other project processes before source recovery")
	}
	defer unlock()
	root, err := os.OpenRoot(prj.Root)
	if err != nil {
		return err
	}
	defer root.Close()
	p := newPipeline(prj.Root, nil, nil)
	j, err := loadSourceJournal(root, p)
	if err != nil {
		return err
	}
	j.checkpoint = checkpoint
	if j.state != "prepared" {
		return j.cleanup()
	}
	// Check every backup and source before changing any of them. Check again
	// immediately before each write; these are conflict guards, not OS CAS.
	for _, e := range j.entries {
		if err := j.checkDirectory(); err != nil {
			return err
		}
		if e.Backup != "" {
			backup, err := readJournalFile(root, path.Join(sourceJournalDir, e.Backup), recoverySourceLimit(e.Path))
			if err != nil || sourceVersionOf(backup) != e.Before {
				return fmt.Errorf("source recovery backup is missing or changed: %s", e.Path)
			}
		}
		got, err := p.recoverySourceBytes(root, e.Path)
		if err != nil || !slices.Contains(j.allowed[e.Path], sourceVersionOf(got)) {
			return fmt.Errorf("%s changed outside the interrupted transaction; refusing recovery", e.Path)
		}
	}
	if checkpoint != nil {
		checkpoint("recovery-validated", "")
	}
	for _, e := range slices.Backward(j.entries) {
		if err := j.checkDirectory(); err != nil {
			return err
		}
		got, err := p.recoverySourceBytes(root, e.Path)
		if err != nil || !slices.Contains(j.allowed[e.Path], sourceVersionOf(got)) {
			return fmt.Errorf("%s changed during recovery; refusing to overwrite it", e.Path)
		}
		if sourceVersionOf(got) == e.Before {
			continue
		}
		var before []byte
		if e.Backup != "" {
			before, err = readJournalFile(root, path.Join(sourceJournalDir, e.Backup), recoverySourceLimit(e.Path))
			if err != nil || sourceVersionOf(before) != e.Before {
				return fmt.Errorf("source recovery backup changed: %s", e.Path)
			}
		}
		if err := j.checkDirectory(); err != nil {
			return err
		}
		if err := replaceSource(root, e.Path, before); err != nil {
			return fmt.Errorf("cannot restore %s: %w", e.Path, err)
		}
		if checkpoint != nil {
			checkpoint("recovery-restored", e.Path)
		}
	}
	return j.finish("restored")
}
