//go:build aego_editor && aego_gltest && linux && cgo && !aego_nonative

package docsview

import (
	"compress/gzip"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"aego/app/playground"
	"aego/engine/docs/democheck"
)

// Evidence contains actual RGBA frames and observations, not just hashes.
func initializeTryEvidence(t *testing.T) {
	t.Helper()
	if os.Getenv("AEGO_DEMOCHECK_EVIDENCE") == "" {
		return
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AEGO_DEMOCHECK_RUN_ID", hex.EncodeToString(id[:]))
}

func writeTryEvidence(name string, value any) error {
	root := os.Getenv("AEGO_DEMOCHECK_EVIDENCE")
	if root == "" {
		return nil
	}
	file := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(file, append(raw, '\n'), 0o644)
}

func writeComponentRegistryEvidence(t *testing.T, p *playground.Playground, site string) {
	t.Helper()
	if os.Getenv("AEGO_DEMOCHECK_EVIDENCE") == "" {
		return
	}
	wasm, err := os.ReadFile(filepath.Join(site, "try", "aego-docs.wasm"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(wasm)
	contracts := make([]democheck.Contract, 0, len(p.Docs().ComponentDemos()))
	for _, component := range p.Docs().ComponentDemos() {
		e, _ := p.Docs().Get(component.ID)
		if e.Demo != nil {
			contracts = append(contracts, *e.Demo)
		}
	}
	record := map[string]any{"schema": 1, "runID": os.Getenv("AEGO_DEMOCHECK_RUN_ID"), "components": p.Docs().ComponentDemos(), "contracts": contracts, "wasmSHA256": hex.EncodeToString(sum[:]), "goVersion": runtime.Version()}
	if err := writeTryEvidence("registry.json", record); err != nil {
		t.Fatal(err)
	}
}

func tryEvidenceDir(c democheck.Contract, env string) (string, string, error) {
	root := os.Getenv("AEGO_DEMOCHECK_EVIDENCE")
	if root == "" {
		return "", "", nil
	}
	if c.DemoID == "" || strings.ContainsAny(c.DemoID, `/\`) || c.DemoID == "." || c.DemoID == ".." {
		return "", "", fmt.Errorf("unsafe Try evidence ID")
	}
	runID := os.Getenv("AEGO_DEMOCHECK_RUN_ID")
	if strings.HasSuffix(c.DemoID, ".negative-observable") {
		root += "-negative"
		runID += "-negative"
	}
	return filepath.Join(root, env, c.DemoID), runID, nil
}

func saveNativeTryObservation(c democheck.Contract, index int, o democheck.Observation) error {
	dir, runID, err := tryEvidenceDir(c, "native")
	if err != nil || dir == "" {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(dir, fmt.Sprintf("observation-%03d.json.gz", index)))
	if err != nil {
		return err
	}
	gz, err := gzip.NewWriterLevel(f, gzip.BestSpeed)
	if err != nil {
		f.Close()
		return err
	}
	record := map[string]any{"schema": 1, "runID": runID, "environment": "native", "demoID": c.DemoID, "observation": o}
	encodeErr := json.NewEncoder(gz).Encode(record)
	gzipErr := gz.Close()
	closeErr := f.Close()
	if encodeErr != nil {
		return encodeErr
	}
	if gzipErr != nil {
		return gzipErr
	}
	return closeErr
}

func saveNegativeTryReport(report democheck.Report) error {
	root := os.Getenv("AEGO_DEMOCHECK_EVIDENCE")
	if root == "" {
		return nil
	}
	root += "-negative"
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	parentID := os.Getenv("AEGO_DEMOCHECK_RUN_ID")
	data, err := json.MarshalIndent(map[string]any{"schema": 1, "runID": parentID + "-negative", "parentRunID": parentID, "purpose": "intentionally false observable must be rejected in every environment", "runtimeReport": report}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "report.json"), append(data, '\n'), 0o644)
}
