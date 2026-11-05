package app

import (
	shaderImporter "aego/asset/importers/shader"
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

// ShaderProblems lists the SHD problems of the last shader imports with the file and line they point at.
func ShaderProblems(imps *pipeline.ImporterRegistry) problems.Source {
	imp, _ := imps.ByID("shader")
	si, _ := imp.(*shaderImporter.Importer)
	return problems.Source{
		Revision: func() uint64 {
			if si == nil {
				return 0
			}
			return si.Revision()
		},
		Append: func(dst []problems.Diagnostic) []problems.Diagnostic {
			if si == nil {
				return dst
			}
			list, _ := si.Diagnostics()
			for _, d := range list {
				dst = append(dst, problems.Diagnostic{
					Owner: OwnerAssets, Code: d.Code, Severity: problems.Error, Message: d.Msg,
					Subject: problems.Subject{Kind: problems.SubjectFile, File: d.File, Line: d.Line},
				})
			}
			return dst
		},
	}
}
