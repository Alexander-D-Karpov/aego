package sdk

import (
	"aego/engine/module"
	"aego/engine/problems"
)

type (
	SectionDesc[T any] = module.SectionDesc[T]
	Sections           = module.Sections
	Diagnostic         = problems.Diagnostic
	ProblemSource      = problems.Source
	ProblemService     = problems.Service
	Subject            = problems.Subject
	AddContext         = module.AddContext
	ValidateContext    = module.ValidateContext
	ChangeSet          = module.ChangeSet
	Change             = module.Change
)

const (
	SeverityInfo    = problems.Info
	SeverityWarning = problems.Warning
	SeverityError   = problems.Error
)

func ProjectSection[T any](r *Registry, d SectionDesc[T]) {
	r.r.ProjectSection(d)
}

func Section[T any](ctx *InitContext, id string) (*T, bool) {
	return module.SectionValue[T](ctx.Sections, id)
}

func Diagnostics(ctx *InitContext) *ProblemService {
	return ctx.Problems
}
