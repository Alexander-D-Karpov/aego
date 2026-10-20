package app

import (
	"aego/asset/pipeline"
	"aego/engine/problems"
)

const OwnerAssets = "assets"

func metaSeverity(k pipeline.MetaIssueKind) problems.Severity {
	switch k {
	case pipeline.MetaDuplicate:
		return problems.Error
	case pipeline.MetaStale:
		return problems.Info
	}
	return problems.Warning
}

func MetaProblems(pl *pipeline.Pipeline) problems.Source {
	return problems.Source{
		Revision: func() uint64 {
			_, rev := pl.MetaIssues()
			return rev
		},
		Append: func(dst []problems.Diagnostic) []problems.Diagnostic {
			list, _ := pl.MetaIssues()
			for _, is := range list {
				dst = append(dst, problems.Diagnostic{
					Owner:    OwnerAssets,
					Code:     is.Code(),
					Severity: metaSeverity(is.Kind),
					Message:  is.Message(),
					Help:     "Assets → Repair Meta Files… will " + is.Action(),
					Subject:  problems.Subject{Kind: problems.SubjectFile, File: is.Path},
				})
			}
			return dst
		},
	}
}
