//go:build !aego_runtime

package docs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"

	"aego/engine/docs/democheck"
	"aego/engine/module"
)

// ComponentDemos inventories exactly the public component entries populated
// from schema.Registry by components(), not functions, diagnostics or guides.
// Field identities are full, stable docs/schema IDs (not localized labels).
func (r *Registry) ComponentDemos() []democheck.Component {
	var out []democheck.Component
	for _, e := range r.entries {
		if e.Component == nil {
			continue
		}
		c := democheck.Component{ID: e.ID, DocumentID: e.ID, Scene: e.ID + "/try"}
		if strings.TrimSpace(e.Try) == "" {
			c.Scene = ""
		} else {
			data, _ := json.Marshal(struct {
				Scene, Select, Script, Code string
				Prefabs                     map[string]string
				Artifacts                   []byte
			}{e.Try, e.TrySelect, e.TryScript, e.TryCode, e.TryPrefabs, e.TryArtifacts})
			sum := sha256.Sum256(data)
			c.Identity = hex.EncodeToString(sum[:])
		}
		for _, field := range r.entries {
			if field.Kind == Field && field.Field != nil && field.Parent == e.ID {
				c.Fields = append(c.Fields, field.ID)
			}
		}
		sort.Strings(c.Fields)
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// CheckComponentDemos is the behavioral gate for the native/web docs test
// harness. A nil runner fails explicitly; static existence is never coverage.
func (r *Registry) CheckComponentDemos(ctx context.Context, runner democheck.Runner) democheck.Report {
	var contracts []democheck.Contract
	for _, e := range r.entries {
		if e.Demo != nil {
			contracts = append(contracts, *e.Demo)
		}
	}
	report := democheck.Check(ctx, r.ComponentDemos(), contracts, runner)
	for _, p := range r.problems {
		if p.Error {
			report.Errors = append(report.Errors, p.ID+": "+p.Message)
		}
	}
	return report
}

// The contract is a sidecar of the existing Try, not a new scene format. The
// same Entry exports it to docs/native clients; prepared pixels remain .aetry.
func (r *Registry) readDemoContract(e *Entry, src module.DocSource, scenePath string) {
	// Guides may reuse a component's Try scene, script and artifacts. The
	// behavioral sidecar belongs only to the component registry entry; a guide
	// is not another owner and must neither attach nor reject that contract.
	if e.Component == nil {
		return
	}
	name := strings.TrimSuffix(path.Clean(scenePath), TryExt) + ".demo.json"
	f, err := src.FS.Open(name)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	problem := func(err error) {
		r.problems = append(r.problems, Problem{ID: e.ID, Error: true, Message: "demo contract " + name + ": " + err.Error()})
	}
	if err != nil {
		problem(err)
		return
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil {
		problem(err)
		return
	}
	if len(b) > 1<<20 {
		problem(errors.New("contract exceeds 1 MiB"))
		return
	}
	var c democheck.Contract
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&c); err != nil {
		problem(err)
		return
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		problem(errors.New("trailing JSON"))
		return
	}
	if c.ComponentID != e.ID || c.DocumentID != e.ID {
		problem(errors.New("component/document identity does not match owning registry entry"))
		return
	}
	if e.Demo != nil {
		problem(errors.New("duplicate component contract"))
		return
	}
	e.Demo = &c
}
