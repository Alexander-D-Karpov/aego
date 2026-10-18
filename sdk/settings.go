package sdk

import (
	"aego/engine"
	"aego/engine/module"
	"aego/engine/problems"
	es "aego/engine/settings"
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

type (
	SettingDesc   = es.Desc
	SettingKey    = es.Key
	SettingChoice = es.Choice
	SettingsStore = es.Store
)

const (
	SettingUser          = es.User
	SettingProject       = es.Project
	SettingRestart       = es.Restart
	SettingTransient     = es.Transient
	SettingAccessibility = es.Accessibility
	SettingConfirm       = es.Confirm
)

func Setting(r *Registry, d SettingDesc) {
	r.r.Setting(d)
}

func Settings(ctx *Context) *SettingsStore {
	s, _ := engine.Service[*es.Store](ctx.Engine, es.ServiceID)
	return s
}
