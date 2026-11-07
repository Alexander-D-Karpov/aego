// Asset Studio artifact builder and static release tooling. This command is
// independent of the desktop editor and docs generator; it reuses the existing
// Go/WASM entrypoint and never reimplements authoring algorithms.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "artstudio-release:", err)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("use build | verify | serve | check-http | check-source | integrate-docs | init | stage | activate | rollback")
	}
	f := flag.NewFlagSet(args[0], flag.ContinueOnError)
	switch args[0] {
	case "build":
		root := f.String("root", ".", "repository root")
		out := f.String("out", ".cache/artstudio-release", "new output directory (must not exist)")
		id := f.String("id", "local", "immutable build ID")
		origin := f.String("app-origin", "http://127.0.0.1:8097", "exact app origin")
		docs := f.String("docs-origin", "http://127.0.0.1:8098", "exact docs origin")
		release := f.Bool("release", false, "require clean exact Git tree and HTTPS")
		epoch := f.Int64("epoch", 0, "SOURCE_DATE_EPOCH; defaults to commit time for release")
		ver := f.String("aego-version", "0.12.1", "compatible Aego base")
		if e := f.Parse(args[1:]); e != nil {
			return e
		}
		m, e := build(buildOptions{*root, *out, *id, *origin, *docs, *ver, *release, *epoch})
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(m)
	case "verify":
		dir := f.String("dir", ".cache/artstudio-release", "artifact directory")
		sha := f.String("sha256", "", "trusted manifest SHA-256")
		if e := f.Parse(args[1:]); e != nil {
			return e
		}
		m, h, e := verify(*dir, *sha)
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"buildID": m.BuildID, "manifestSHA256": h, "files": len(m.Files), "release": m.Release, "status": "PASS"})
	case "check-source":
		dir := f.String("dir", ".cache/artstudio-release", "exact artifact")
		root := f.String("root", ".", "tested source checkout")
		if e := f.Parse(args[1:]); e != nil {
			return e
		}
		m, _, e := verify(*dir, "")
		if e != nil {
			return e
		}
		h, e := sourceHash(*root)
		if e != nil {
			return e
		}
		if h != m.SourceSHA256 {
			return fmt.Errorf("source tree differs from built artifact")
		}
		if m.Release {
			b, e := command(*root, nil, "git", "rev-parse", "HEAD")
			if e != nil {
				return e
			}
			if strings.TrimSpace(string(b)) != m.Commit {
				return fmt.Errorf("HEAD changed since build")
			}
			b, e = command(*root, nil, "git", "status", "--porcelain=v1", "--untracked-files=all")
			if e != nil {
				return e
			}
			if len(bytes.TrimSpace(b)) != 0 {
				return fmt.Errorf("release source no longer clean")
			}
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "PASS", "sourceSHA256": h, "commit": m.Commit})

	case "integrate-docs":
		dir := f.String("dir", "", "verified release artifact")
		out := f.String("out", "", "existing generated documentation directory")
		sha := f.String("sha256", "", "trusted manifest SHA-256")
		if e := f.Parse(args[1:]); e != nil {
			return e
		}
		result, e := integrateDocs(*dir, *out, *sha)
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(result)

	case "serve":
		dir := f.String("dir", ".cache/artstudio-release", "artifact directory")
		addr := f.String("addr", "127.0.0.1:8097", "loopback app address")
		docs := f.String("docs-addr", "127.0.0.1:8098", "loopback docs address, empty to disable")
		if e := f.Parse(args[1:]); e != nil {
			return e
		}
		return serve(*dir, *addr, *docs)
	case "check-http":
		dir := f.String("dir", ".cache/artstudio-release", "local reference artifact")
		base := f.String("url", "", "base URL ending in /")
		sha := f.String("sha256", "", "trusted manifest SHA-256")
		if e := f.Parse(args[1:]); e != nil {
			return e
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		v, e := httpCheck(ctx, *base, *dir, *sha)
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(v)
	case "init", "stage", "activate", "rollback":
		return deployCLI(args)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}
