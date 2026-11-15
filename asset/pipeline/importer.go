package pipeline

import (
	"context"
	"path/filepath"
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
	Deps      []ids.AssetID
}

type Result struct {
	Subs        []SubResult
	Kind        asset.ArtifactKind
	Artifacts   []Produced
	Deps        []ids.AssetID
	Group       string
	Raw         *texture.Data
	RawFilter   int
	RawSemantic asset.TextureSemantic
	RawPivot    [2]float32
	RawHasPivot bool
	RawNormal   ids.AssetID
	RawEmission ids.AssetID
	Source      []byte
	Error       error
}

type ImportContext struct {
	ID     ids.AssetID
	Path   string
	Source []byte
	Meta   *Meta
	Log    *log.ChannelLogger
	Lookup func(path string) (ids.AssetID, bool)
	Read   func(path string) ([]byte, error)
	// Source returns a source path for a stable ID, including sub-assets as
	// sheet.png#frame. It never searches by a derived file name at runtime.
	SourcePath func(id ids.AssetID) (string, bool)
}

type Importer interface {
	ID() string
	Version() int
	Extensions() []string
	Defaults() Settings
	Import(ctx *ImportContext) (*Result, error)
}

// SourceEditLimiter lets a trusted registered importer request a specific
// bound for guarded source transactions. It does not change the limit for
// metadata or unrelated formats. The pipeline applies its own maximum too.
type SourceEditLimiter interface {
	SourceEditLimit() int64
}

// CacheSalter adds artifact-less inputs (e.g. embedded includes and validator version) before cache lookup; errors must not reuse older successful artifacts.
type CacheSalter interface {
	CacheSalt(ctx *ImportContext) ([]byte, error)
}

// SourceDependencySalter says CacheSalt covers all current dependency sources.
// Such importers store references, not compiled dependency data, and cannot depend on artifact import order.
type SourceDependencySalter interface {
	CacheSalter
	SourceDependencies() bool
}

// MetadataResolver resolves authoring conveniences to stable references before computing the import key, returning cloned settings only when an update is needed.
// The pipeline owns atomic persistence and checks concurrent edits; Preview calls the hook on a private copy without writing files.
type MetadataResolver interface {
	ResolveMetadata(ctx *ImportContext) (Settings, error)
}

// GuardedMetadataPreviewer renders an indexed source with a private metadata
// snapshot, including a newer format only for preview. It does not authorize
// normal import or persistence. The pipeline verifies that Source matches the
// indexed disk source before and after the call. The caller must separately
// guard document revision and generation when using an asynchronously produced
// result. The renderer runs without the pipeline lock: it receives only ID,
// Path, Source and Log, and must treat Source and metadata as immutable. Meta
// and pipeline callbacks are unavailable; dependent assets require a separate
// guarded snapshot.
type GuardedMetadataPreviewer interface {
	GuardedMetadataCandidate(path string, metadata []byte) (bool, error)
	PreviewGuardedMetadata(ctx context.Context, input *ImportContext, metadata []byte) (*Result, error)
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

var _ = vfs.Mount{}

type DevEnv struct {
	Workspace any
}
