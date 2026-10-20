package dataasset

import (
	"unsafe"

	"aego/data"
	"aego/engine/module"
	"aego/engine/problems"
	"aego/schema"
)

const (
	codeBroken  = "DAT001"
	codeType    = "DAT004"
	codeContent = "DAT005"
)

func broken(code, msg string) []problems.Diagnostic {
	return []problems.Diagnostic{{Code: code, Severity: problems.Error, Message: msg, BlocksPlay: true}}
}

func report(out []problems.Diagnostic, c *module.AssetCheck, td *schema.TypeDescriptor, p unsafe.Pointer, issues []string, prefix string) []problems.Diagnostic {
	for _, s := range issues {
		out = append(out, problems.Diagnostic{Code: codeContent, Severity: problems.Warning, Message: s})
	}
	for _, is := range data.Validate(td, p, c.Exists) {
		out = append(out, problems.Diagnostic{Code: is.Code, Severity: problems.Warning, Message: prefix + is.Message,
			Subject: problems.Subject{Kind: problems.SubjectAsset, Asset: c.ID, File: c.Path, Field: prefix + is.Field}})
	}
	return append(out, module.ValidateData(td, p)...)
}

func knownType(c *module.AssetCheck, name string) ([]problems.Diagnostic, bool) {
	if td, ok := c.Schema.ByName(name); ok && td.Kind == schema.TypeAsset {
		return nil, true
	}
	if c.Partial {
		return nil, false
	}
	return broken(codeType, "unknown data type "+name+"; the module that registers it is not loaded"), false
}

func checkRecord(c *module.AssetCheck) []problems.Diagnostic {
	doc, err := data.ParseDoc(c.Source, c.Path)
	if err != nil {
		return broken(codeBroken, err.Error())
	}
	if ds, ok := knownType(c, doc.Type); !ok {
		return ds
	}
	rec, err := data.NewLibrary(c.Schema, c.Data).Record(c.ID)
	if err != nil {
		return broken(codeBroken, err.Error())
	}
	return report(nil, c, rec.Type, rec.Ptr, rec.Issues, "")
}

func checkTable(c *module.AssetCheck) []problems.Diagnostic {
	doc, err := data.ParseTable(c.Source, c.Path)
	if err != nil {
		return broken(codeBroken, err.Error())
	}
	if ds, ok := knownType(c, doc.Type); !ok {
		return ds
	}
	t, err := data.NewLibrary(c.Schema, c.Data).Table(c.ID)
	if err != nil {
		return broken(codeBroken, err.Error())
	}
	var out []problems.Diagnostic
	for _, s := range t.Issues {
		out = append(out, problems.Diagnostic{Code: codeContent, Severity: problems.Warning, Message: s})
	}
	for i, p := range t.Rows {
		out = report(out, c, t.Type, p, nil, t.IDs[i]+".")
	}
	return out
}
