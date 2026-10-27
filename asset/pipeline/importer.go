package pipeline

import (
	"path/filepath"
	"sort"
	"strings"

	"aego/asset"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/render/texture"
	"aego/vfs"
)

type Produced struct {
	Kind asset.ArtifactKind
	Name string
	Data []byte
}

// SubResult is a sub-asset an import produces next to its main entry, such as a cell of a sliced sheet (D339).
type SubResult struct {
	ID        ids.AssetID
	Name      string
	Kind      asset.ArtifactKind
	Artifacts []Produced
}

type Result struct {
	Subs      []SubResult
	Kind      asset.ArtifactKind
	Artifacts []Produced
	Deps      []ids.AssetID
	Group     string
	Raw       *texture.Data
	RawFilter int
	Source    []byte
	Error     error
}

type ImportContext struct {
	ID     ids.AssetID
	Path   string
	Source []byte
	Meta   *Meta
	Log    *log.ChannelLogger
	Lookup func(path string) (ids.AssetID, bool)
	Read   func(path string) ([]byte, error)
}

type Importer interface {
	ID() string
	Version() int
	Extensions() []string
	Defaults() Settings
	Import(ctx *ImportContext) (*Result, error)
}

type GroupMember struct {
	ID       ids.AssetID
	Name     string
	Path     string
	Settings Settings
	Result   *Result
}

type GroupContext struct {
	Group    string
	PageID   ids.AssetID
	Log      *log.ChannelLogger
	Previous func(name string) ([]byte, bool)
}

type GroupImporter interface {
	Importer
	GroupVersion() int
	Finalize(ctx *GroupContext, members []GroupMember) (map[ids.AssetID]*Result, []Produced, error)
}

type ImporterRegistry struct {
	byID  map[string]Importer
	byExt map[string]Importer
	order []Importer
}

func NewImporterRegistry() *ImporterRegistry {
	return &ImporterRegistry{
		byID:  map[string]Importer{},
		byExt: map[string]Importer{},
	}
}

func (r *ImporterRegistry) Register(imp Importer) error {
	id := imp.ID()
	if id == "" {
		return errs.New(errs.AssetBadImporter, "importer needs an id")
	}
	if _, dup := r.byID[id]; dup {
		return errs.New(errs.AssetBadImporter, "importer id is already registered", log.F("importer", id))
	}
	for _, ext := range imp.Extensions() {
		ext = strings.ToLower(ext)
		if other, dup := r.byExt[ext]; dup {
			return errs.New(errs.AssetBadImporter, "extension is already claimed",
				log.F("ext", ext), log.F("importer", id), log.F("other", other.ID()))
		}
		r.byExt[ext] = imp
	}
	r.byID[id] = imp
	r.order = append(r.order, imp)
	return nil
}

func (r *ImporterRegistry) ByID(id string) (Importer, bool) {
	imp, ok := r.byID[id]
	return imp, ok
}

func (r *ImporterRegistry) ForPath(path string) (Importer, bool) {
	imp, ok := r.byExt[strings.ToLower(filepath.Ext(path))]
	return imp, ok
}

func (r *ImporterRegistry) Each(fn func(Importer)) {
	for _, imp := range r.order {
		fn(imp)
	}
}

func sortedKeys(s Settings) []string {
	out := make([]string, 0, len(s))
	for k := range s {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

var _ = vfs.Mount{}

type DevEnv struct {
	Workspace any
}
