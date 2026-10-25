package animctrl

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
)

const (
	Version   = 2
	Extension = ".aectrl"
	CodeParam = "ANC001"
	CodeState = "ANC002"
	CodeSelf  = "ANC003"
	CodeLoop  = "ANC005"
	CodeText  = "ANC006"
)

type Importer struct{}

func New() *Importer {
	return &Importer{}
}

func (*Importer) ID() string                  { return "animctrl" }
func (*Importer) Version() int                { return Version }
func (*Importer) Extensions() []string        { return []string{Extension} }
func (*Importer) Defaults() pipeline.Settings { return pipeline.Settings{} }

func fail(ctx *pipeline.ImportContext, code, msg string) error {
	if code != "" {
		msg = code + ": " + msg
	}
	return errs.New(errs.AssetImportFailed, msg, log.F("path", ctx.Path))
}

func (*Importer) Import(ctx *pipeline.ImportContext) (*pipeline.Result, error) {
	doc, err := aetext.Parse(ctx.Source, ctx.Path)
	if err != nil {
		return nil, errs.Wrap(err, errs.AssetImportFailed, "cannot parse the controller").With(log.F("path", ctx.Path))
	}
	g, probs, err := anim.DecodeGraph(doc)
	if err != nil {
		return nil, fail(ctx, CodeText, err.Error())
	}
	if len(probs) == 0 {
		probs = g.Check(probs)
	}
	if len(probs) > 0 {
		code, msg := describe(g, probs[0])
		return nil, fail(ctx, code, msg)
	}
	g.Layout()
	return &pipeline.Result{
		Kind:      asset.ArtCtrl,
		Deps:      deps(g, ctx.ID),
		Artifacts: []pipeline.Produced{{Kind: asset.ArtCtrl, Data: anim.MarshalGraph(g)}},
	}, nil
}

func where(g *anim.Graph, i int) string {
	if i < 0 || i >= len(g.Transitions) {
		return ""
	}
	return "transition " + strconv.Itoa(i+1) + ": "
}

func describe(g *anim.Graph, p anim.GraphProblem) (string, string) {
	at := where(g, p.Transition)
	switch p.Kind {
	case anim.GraphEmpty:
		return CodeState, "the controller has no states"
	case anim.GraphState:
		if p.Name != "" {
			return CodeState, at + "there is no state " + p.Name
		}
		return CodeState, at + "names a state that does not exist"
	case anim.GraphParam:
		if p.Name != "" {
			return CodeParam, at + "there is no parameter " + p.Name
		}
		return CodeParam, at + "a condition does not fit the kind of its parameter"
	case anim.GraphSelf:
		return CodeSelf, at + "a transition into its own state needs exitTime"
	}
	return CodeLoop, "transitions without duration, conditions or exit time form a loop"
}

func deps(g *anim.Graph, self ids.AssetID) []ids.AssetID {
	var out []ids.AssetID
	for _, s := range g.States {
		if id := s.Clip; !id.IsZero() && id != self && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	slices.SortFunc(out, func(a, b ids.AssetID) int { return strings.Compare(a.String(), b.String()) })
	return out
}
