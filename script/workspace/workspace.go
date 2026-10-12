package workspace

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"aego/script/abi"
	"aego/script/ast"
	"aego/script/bind/manifest"
	"aego/script/bytecode"
	"aego/script/checker"
	"aego/script/diag"
	"aego/script/parser"
	"aego/script/source"
)

const Ext = ".aes"

type Config struct {
	Root     string
	FS       fs.FS
	Module   string
	Scripts  string
	Bindings *abi.Table
	Docs     *manifest.Manifest
	Debug    bool
	Tests    bool
}

type Snapshot struct {
	Gen         uint64
	Analysis    *checker.Analysis
	Sources     *source.Manager
	Files       map[source.FileID]*ast.File
	Packages    map[source.FileID]string
	Diagnostics []diag.Diagnostic
	Bindings    *abi.Table
	Docs        *manifest.Manifest
}

func (s *Snapshot) Errors() int {
	n, _ := diag.Count(s.Diagnostics)
	return n
}

type Generation struct {
	ID      uint64
	Modules map[string]*bytecode.Module
	Hashes  map[string][32]byte
	ABIHash [32]byte
	Changed []string
	Failed  map[string][]diag.Diagnostic
	Sources *source.Manager
}

type entry struct {
	id      source.FileID
	path    string
	pkg     string
	test    bool
	disk    []byte
	onDisk  bool
	over    []byte
	hasOver bool
}

type built struct {
	key   [32]byte
	iface [32]byte
	mod   *bytecode.Module
}

type Workspace struct {
	mu        sync.Mutex
	bmu       sync.Mutex
	cfg       Config
	fs        fs.FS
	scripts   string
	files     map[string]*entry
	order     []*entry
	ver       uint64
	snap      *Snapshot
	empty     *abi.Table
	state     map[string]built
	committed *Generation
	nextID    uint64
}

func newGeneration() *Generation {
	return &Generation{
		Modules: map[string]*bytecode.Module{},
		Hashes:  map[string][32]byte{},
		Failed:  map[string][]diag.Diagnostic{},
		Sources: source.NewManager(),
	}
}

func Open(cfg Config) (*Workspace, error) {
	if cfg.FS == nil {
		root, err := filepath.Abs(cfg.Root)
		if err != nil {
			return nil, err
		}
		cfg.Root = root
		cfg.FS = os.DirFS(root)
	}
	scripts, err := scriptsDir(cfg)
	if err != nil {
		return nil, err
	}
	if cfg.Module == "" {
		cfg.Module = "game"
	}
	empty := abi.NewTable()
	empty.Freeze()
	w := &Workspace{
		cfg:       cfg,
		fs:        cfg.FS,
		scripts:   scripts,
		files:     map[string]*entry{},
		empty:     empty,
		state:     map[string]built{},
		committed: newGeneration(),
	}
	if err := w.Rescan(); err != nil {
		return nil, err
	}
	return w, nil
}

func scriptsDir(cfg Config) (string, error) {
	s := cfg.Scripts
	switch {
	case s == "":
		return "scripts", nil
	case filepath.IsAbs(s):
		if cfg.Root == "" {
			return "", errors.New("an absolute scripts directory needs a workspace root")
		}
		r, err := filepath.Rel(cfg.Root, s)
		if err != nil {
			return "", err
		}
		s = r
	}
	s = path.Clean(filepath.ToSlash(s))
	if s == ".." || strings.HasPrefix(s, "../") || strings.HasPrefix(s, "/") {
		return "", errors.New("scripts directory " + cfg.Scripts + " is outside the workspace")
	}
	return s, nil
}

func (w *Workspace) Module() string {
	return w.cfg.Module
}

func (w *Workspace) Root() string {
	return w.cfg.Root
}

// Version is the current edit generation; it advances on every mutation and
// matches the Gen of the snapshot produced for that state.
func (w *Workspace) Version() uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.ver
}

func (w *Workspace) rel(p string) string {
	if filepath.IsAbs(p) && w.cfg.Root != "" {
		if r, err := filepath.Rel(w.cfg.Root, p); err == nil {
			p = r
		}
	}
	return path.Clean(filepath.ToSlash(p))
}

func (w *Workspace) PackageOf(p string) (string, bool) {
	r := w.rel(p)
	if !strings.HasSuffix(r, Ext) {
		return "", false
	}
	dir := path.Dir(r)
	switch {
	case dir == w.scripts:
		return w.cfg.Module, true
	case strings.HasPrefix(dir, w.scripts+"/"):
		return w.cfg.Module + "/" + strings.TrimPrefix(dir, w.scripts+"/"), true
	}
	return "", false
}

func (w *Workspace) entryFor(r string) *entry {
	if e, ok := w.files[r]; ok {
		return e
	}
	pkg, _ := w.PackageOf(r)
	e := &entry{
		id:   source.FileID(len(w.order) + 1),
		path: r,
		pkg:  pkg,
		test: IsTest(r),
	}
	w.files[r] = e
	w.order = append(w.order, e)
	return e
}

func (w *Workspace) touch() {
	w.ver++
	w.snap = nil
}

func (w *Workspace) Rescan() error {
	found := map[string][]byte{}
	err := fs.WalkDir(w.fs, w.scripts, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && p == w.scripts {
				return fs.SkipDir
			}
			return err
		}
		if d.IsDir() {
			if p != w.scripts && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if path.Ext(p) != Ext {
			return nil
		}
		src, err := fs.ReadFile(w.fs, p)
		if err != nil {
			return err
		}
		found[p] = src
		return nil
	})
	if err != nil {
		return err
	}
	w.Sync(found)
	return nil
}

func (w *Workspace) Sync(files map[string][]byte) {
	w.mu.Lock()
	defer w.mu.Unlock()
	seen := make(map[string]bool, len(files))
	for p, src := range files {
		r := w.rel(p)
		seen[r] = true
		e := w.entryFor(r)
		e.disk, e.onDisk = src, true
	}
	for _, e := range w.order {
		if e.onDisk && !seen[e.path] {
			e.disk, e.onDisk = nil, false
		}
	}
	w.touch()
}

func (w *Workspace) Changed(paths []string) {
	type read struct {
		path string
		src  []byte
		ok   bool
	}
	var reads []read
	for _, p := range paths {
		r := w.rel(p)
		if !strings.HasSuffix(r, Ext) {
			continue
		}
		src, err := fs.ReadFile(w.fs, r)
		reads = append(reads, read{path: r, src: src, ok: err == nil})
	}
	if len(reads) == 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, rd := range reads {
		e := w.entryFor(rd.path)
		if rd.ok {
			e.disk, e.onDisk = rd.src, true
		} else {
			e.disk, e.onDisk = nil, false
		}
	}
	w.touch()
}

func (w *Workspace) Removed(paths []string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, p := range paths {
		if e, ok := w.files[w.rel(p)]; ok {
			e.disk, e.onDisk = nil, false
		}
	}
	w.touch()
}

func (w *Workspace) SetOverlay(p string, src []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()
	e := w.entryFor(w.rel(p))
	e.over, e.hasOver = append([]byte(nil), src...), true
	w.touch()
}

func (w *Workspace) ClearOverlay(p string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if e, ok := w.files[w.rel(p)]; ok && e.hasOver {
		e.over, e.hasOver = nil, false
		w.touch()
	}
}

func (w *Workspace) SetBindings(t *abi.Table) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.cfg.Bindings = t
	w.touch()
}

func (w *Workspace) SetDocs(m *manifest.Manifest) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.cfg.Docs = m
	w.touch()
}

func (w *Workspace) table() *abi.Table {
	if w.cfg.Bindings != nil {
		return w.cfg.Bindings
	}
	return w.empty
}

func (w *Workspace) Bindings() *abi.Table {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.table()
}

func (w *Workspace) Docs() *manifest.Manifest {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.cfg.Docs
}

func (w *Workspace) FileID(p string) (source.FileID, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	e, ok := w.files[w.rel(p)]
	if !ok {
		return source.NoFile, false
	}
	return e.id, true
}

func (w *Workspace) Path(id source.FileID) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	if id == source.NoFile || int(id) > len(w.order) {
		return ""
	}
	return w.order[id-1].path
}

func (w *Workspace) Packages() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	set := map[string]bool{}
	for _, e := range w.order {
		if e.pkg != "" && (e.onDisk || e.hasOver) {
			set[e.pkg] = true
		}
	}
	return sortedKeys(set)
}

func (w *Workspace) Sources() *source.Manager {
	return w.Snapshot().Sources
}

func (w *Workspace) Committed() *Generation {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.committed
}

type input struct {
	sm    *source.Manager
	pkgs  map[string][]source.FileID
	srcs  map[string]map[string][]byte
	pkgOf map[source.FileID]string
}

func (w *Workspace) inputs(overlay, tests bool) *input {
	in := &input{
		sm:    source.NewManager(),
		pkgs:  map[string][]source.FileID{},
		srcs:  map[string]map[string][]byte{},
		pkgOf: map[source.FileID]string{},
	}
	for _, e := range w.order {
		src, live := e.disk, e.onDisk
		if overlay && e.hasOver {
			src, live = e.over, true
		}
		if !live {
			src = nil
		}
		id := in.sm.Add(e.path, src)
		if !live || e.pkg == "" || (e.test && !tests) {
			continue
		}
		in.pkgs[e.pkg] = append(in.pkgs[e.pkg], id)
		in.pkgOf[id] = e.pkg
		if in.srcs[e.pkg] == nil {
			in.srcs[e.pkg] = map[string][]byte{}
		}
		in.srcs[e.pkg][e.path] = src
	}
	return in
}

func (in *input) analyze(tab *abi.Table, module string) (*checker.Analysis, map[source.FileID]*ast.File) {
	files := make(map[string][]*ast.File, len(in.pkgs))
	byID := make(map[source.FileID]*ast.File)
	var ds []diag.Diagnostic
	for pkg, list := range in.pkgs {
		for _, id := range list {
			f, d := parser.ParseFile(in.sm, id)
			ds = append(ds, d...)
			files[pkg] = append(files[pkg], f)
			byID[id] = f
		}
	}
	a := checker.Check(in.sm, files, tab, checker.Options{Module: module})
	a.Diagnostics = append(ds, a.Diagnostics...)
	diag.Sort(a.Diagnostics)
	return a, byID
}

func (w *Workspace) Snapshot() *Snapshot {
	w.mu.Lock()
	if s := w.snap; s != nil && s.Gen == w.ver {
		w.mu.Unlock()
		return s
	}
	ver := w.ver
	in := w.inputs(true, w.cfg.Tests)
	tab, docs, module := w.table(), w.cfg.Docs, w.cfg.Module
	w.mu.Unlock()
	a, files := in.analyze(tab, module)
	s := &Snapshot{
		Gen:         ver,
		Analysis:    a,
		Sources:     in.sm,
		Files:       files,
		Packages:    in.pkgOf,
		Diagnostics: a.Diagnostics,
		Bindings:    tab,
		Docs:        docs,
	}
	w.mu.Lock()
	if w.ver == ver {
		w.snap = s
	}
	w.mu.Unlock()
	return s
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
