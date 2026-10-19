package services

import "aego/engine/problems"

type ExportRequest struct {
	Target  string
	Profile string
	Out     string
}

type ExportReport struct {
	Out     string
	Lines   [][2]string
	Issues  []problems.Diagnostic
	Blocked bool
}

type Exporter interface {
	Targets() []string
	Out(target, profile string) string
	Prepare() any
	Check(prep any, profile string) []problems.Diagnostic
	Export(prep any, req ExportRequest, progress func(float32, string)) (ExportReport, error)
	Reveal(path string) error
}
