package animedit

import (
	"fmt"

	"aego/editor/core/document"
)

// WithDocuments protects the existing scene-bound Timeline/Animator file
// transactions from overwriting a dirty standalone source view. Clean views
// adopt successful timeline writes through the same DocumentManager buffer.
func WithDocuments(files Files, docs *document.Manager) Files {
	return documentFiles{Files: files, docs: docs}
}

type documentFiles struct {
	Files
	docs *document.Manager
}

func (f documentFiles) Write(rel string, data []byte) error {
	var d *document.ScriptDocument
	if f.docs != nil {
		d = f.docs.Script(rel)
	}
	if d != nil && (d.Dirty() || d.Conflict) {
		return fmt.Errorf("animation source has unsaved or conflicting edits: %s; save or reload it before editing the timeline", rel)
	}
	if err := f.Files.Write(rel, data); err != nil {
		return err
	}
	if d != nil {
		return f.docs.ReloadScript(d)
	}
	return nil
}
