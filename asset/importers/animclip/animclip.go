package animclip

import (
	"slices"
	"strconv"
	"strings"

	"aego/anim"
	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/format/aetext"
	"aego/schema"
)

const (
	Version   = 1
	Extension = ".aeanim"
	CodeTrack = "ANM001"
	CodeRange = "ANM002"
	CodeEvent = "ANM003"
)

type Importer struct{}

func New() *Importer {
	return &Importer{}
}

func (*Importer) ID() string                  { return "animclip" }
func (*Importer) Version() int                { return Version }
func (*Importer) Extensions() []string        { return []string{Extension} }
func (*Importer) Defaults() pipeline.Settings { return pipeline.Settings{} }

func fail(ctx *pipeline.ImportContext, code, msg string) error {
	return errs.New(errs.AssetImportFailed, code+": "+msg, log.F("path", ctx.Path))
}

func (*Importer) Import(ctx *pipeline.ImportContext) (*pipeline.Result, error) {
	doc, err := aetext.Parse(ctx.Source, ctx.Path)
	if err != nil {
		return nil, errs.Wrap(err, errs.AssetImportFailed, "cannot parse the clip").With(log.F("path", ctx.Path))
	}
	c, err := anim.Decode(doc)
	if err != nil {
		return nil, fail(ctx, CodeTrack, err.Error())
	}
	if ps := c.Check(nil); len(ps) > 0 {
		code, msg := describe(c, ps[0])
		return nil, fail(ctx, code, msg)
	}
	return &pipeline.Result{
		Kind:      asset.ArtAnim,
		Deps:      deps(c, ctx.ID),
		Artifacts: []pipeline.Produced{{Kind: asset.ArtAnim, Data: anim.Marshal(c)}},
	}, nil
}

func trackName(t *anim.Track) string {
	if t.Target == "" {
		return t.Property
	}
	return t.Target + ": " + t.Property
}

func at(t float32) string {
	return strconv.FormatFloat(float64(t), 'g', -1, 32)
}

func describe(c *anim.Clip, p anim.Problem) (string, string) {
	switch p.Kind {
	case anim.ProblemLength:
		return CodeRange, "the clip length must be positive"
	case anim.ProblemRoot:
		return CodeTrack, "track " + trackName(&c.Tracks[p.Track]) + ": root motion needs the node's own transform.position with kind vec2"
	case anim.ProblemBadKind:
		t := &c.Tracks[p.Track]
		return CodeTrack, "track " + trackName(t) + " has kind " + t.Kind.String() + ", which cannot be animated"
	case anim.ProblemEmpty:
		return CodeTrack, "track " + trackName(&c.Tracks[p.Track]) + " has no keys"
	case anim.ProblemKeyRange:
		return CodeRange, "track " + trackName(&c.Tracks[p.Track]) + " has a key at " + at(p.T) + ", outside 0.." + at(c.Length)
	case anim.ProblemEventRange:
		return CodeRange, "event " + c.Events[p.Event].Name + " at " + at(p.T) + " is outside 0.." + at(c.Length)
	}
	return CodeEvent, "an event has no name"
}

func deps(c *anim.Clip, self ids.AssetID) []ids.AssetID {
	var out []ids.AssetID
	for i := range c.Tracks {
		t := &c.Tracks[i]
		if t.Kind != schema.KindAssetRef {
			continue
		}
		for _, k := range t.Keys {
			if id := k.V.A; !id.IsZero() && id != self && !slices.Contains(out, id) {
				out = append(out, id)
			}
		}
	}
	slices.SortFunc(out, func(a, b ids.AssetID) int { return strings.Compare(a.String(), b.String()) })
	return out
}
