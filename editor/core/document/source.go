package document

import (
	"io"
	"io/fs"
	"os"

	"aego/core/errs"
	"aego/core/log"
)

// OpenSource opens the saved project source without creating a document or
// reading its working journal. Callers own the file and may impose a format's
// streaming/size limits before decoding it.
func (m *Manager) OpenSource(rel string) (fs.File, error) {
	rel = Clean(rel)
	if !validProjectPath(rel) {
		return nil, errs.New(errs.EditorDocument, "source path must stay inside the project")
	}
	f, err := os.OpenInRoot(m.Abs("."), rel)
	if err != nil {
		return nil, errs.Wrap(err, errs.EditorDocument, "cannot open project source").With(log.F("path", rel))
	}
	return f, nil
}

// ReadSource reads saved source bytes, including a sidecar whose open document
// has unsaved edits. Restoring runtime artifacts must not read that dirty buffer.
func (m *Manager) ReadSource(rel string) ([]byte, error) {
	f, err := m.OpenSource(rel)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, errs.Wrap(err, errs.EditorDocument, "cannot read project source").With(log.F("path", rel))
	}
	return data, nil
}
