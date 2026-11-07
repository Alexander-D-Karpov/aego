// Package cli exposes the data-only remote example transport. It never starts
// Aego, a shell, a module installer, or a project script.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"aego/project/remoteexample"
)

type copyResult struct {
	ID          string `json:"id"`
	ExampleID   string `json:"exampleID"`
	Version     string `json:"version"`
	Directory   string `json:"directory"`
	EntryScene  string `json:"entryScene"`
	ManifestURL string `json:"manifestURL"`
}

func result(c remoteexample.Copy) copyResult {
	return copyResult{c.ID, c.Manifest.ExampleID, c.Manifest.Version, c.Directory, c.Entry, c.ManifestURL}
}

// Options supplies the actual host compatibility data. The standalone tool can
// explicitly opt into overrides; the installed editor CLI cannot misrepresent
// its engine or built-in module set.
type Options struct {
	CommandName                 string
	EngineVersion               string
	InstalledModules            []string
	AllowCompatibilityOverrides bool
	HelpText                    string
}

// HelpText is localized by the installed CLI; the standalone tool uses it as-is.
const HelpText = `Remote examples (data only; no code execution)
  remoteexample clone  -url HTTPS_MANIFEST -approve-origin HTTPS_ORIGIN [-root DIR]
  remoteexample open   -url HTTPS_MANIFEST -approve-origin HTTPS_ORIGIN [-root DIR]
  remoteexample recent [-root DIR]
  remoteexample reopen -id LOCAL_COPY_ID [-root DIR]
  remoteexample check  -id LOCAL_COPY_ID -approve-origin HTTPS_ORIGIN [-root DIR]
  remoteexample link   -url HTTPS_MANIFEST [-mode project|scene]
  remoteexample inspect -link AEGO_URL

clone always creates an independent editable copy. open reuses the most recent
local copy, including offline edits, before attempting HTTPS. check only reports
updates; it never replaces files. inspect parses a deep link but grants no trust.
No origin is implicitly trusted. -approve-origin must be an exact HTTPS origin.`

func Run(ctx context.Context, args []string, out, diagnostics io.Writer, options Options) error {
	name := options.CommandName
	if name == "" {
		name = "remoteexample"
	}
	if options.EngineVersion == "" {
		return fmt.Errorf("actual engine version is required")
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		help := options.HelpText
		if help == "" {
			help = HelpText
		}
		if options.AllowCompatibilityOverrides {
			help += "\nUse -engine and -modules to supply the actual installed engine/module set."
		}
		_, err := fmt.Fprintln(out, strings.ReplaceAll(help, "remoteexample ", name+" "))
		return err
	}
	command := args[0]
	switch command {
	case "clone", "open", "recent", "reopen", "check", "link", "inspect":
	default:
		return fmt.Errorf("unknown operation %q", command)
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	root := flags.String("root", "", "persistent store (default: user config/aego/remote-examples)")
	source := flags.String("url", "", "HTTPS manifest URL")
	approval := flags.String("approve-origin", "", "explicitly confirmed exact HTTPS origin")
	current := options.EngineVersion
	modules := strings.Join(options.InstalledModules, ",")
	if options.AllowCompatibilityOverrides {
		flags.StringVar(&current, "engine", current, "installed engine version")
		flags.StringVar(&modules, "modules", modules, "comma-separated installed module IDs")
	}
	id := flags.String("id", "", "local copy ID")
	link := flags.String("link", "", "untrusted aego://example deep link to inspect")
	mode := flags.String("mode", "project", "deep-link operation: project or scene")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	encode := func(v any) error { e := json.NewEncoder(out); e.SetIndent("", "  "); return e.Encode(v) }
	if command == "inspect" {
		parsed, err := remoteexample.ParseLink(*link)
		if err != nil {
			return err
		}
		origin, err := remoteexample.Origin(parsed.ManifestURL)
		if err != nil {
			return err
		}
		return encode(struct {
			ManifestURL string `json:"manifestURL"`
			Mode        string `json:"mode"`
			Origin      string `json:"origin"`
			Approved    bool   `json:"approved"`
		}{parsed.ManifestURL, parsed.Mode, origin, false})
	}
	if command == "link" {
		encoded, err := remoteexample.MakeLink(*source, *mode)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, encoded)
		return err
	}
	if *approval != "" {
		normalized, err := remoteexample.Origin(*approval)
		if err != nil || normalized != *approval {
			return fmt.Errorf("approval must be an exact HTTPS origin, not a path or wildcard")
		}
	}
	if command == "clone" || command == "open" {
		origin, err := remoteexample.Origin(*source)
		if err != nil {
			return err
		}
		if command == "clone" && *approval != origin {
			return fmt.Errorf("explicit confirmation required for origin %s", origin)
		}
	}
	if (command == "check" || command == "reopen") && *id == "" {
		return fmt.Errorf("-id is required")
	}
	if *root == "" {
		config, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		*root = filepath.Join(config, "aego", "remote-examples")
	}
	store, err := remoteexample.NewStore(*root, remoteexample.Limits{})
	if err != nil {
		return err
	}
	switch command {
	case "recent":
		copies, err := store.Copies()
		if err != nil {
			return err
		}
		rows := make([]copyResult, 0, len(copies))
		for _, c := range copies {
			rows = append(rows, result(c))
		}
		return encode(rows)
	case "reopen":
		c, err := store.Reopen(*id)
		if err != nil {
			return err
		}
		return encode(result(c))
	case "open":
		c, err := store.OpenRecent(*source)
		if err == nil {
			return encode(result(c))
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	var installed []string
	for _, s := range strings.Split(modules, ",") {
		if s = strings.TrimSpace(s); s != "" {
			installed = append(installed, s)
		}
	}
	client, err := remoteexample.NewClient(remoteexample.Policy{EngineVersion: current, InstalledModules: installed})
	if err != nil {
		return err
	}
	defer client.Close()
	if command == "check" {
		c, err := store.Reopen(*id)
		if err != nil {
			return err
		}
		update, err := client.CheckUpdate(ctx, c, *approval)
		if err != nil {
			return err
		}
		return encode(update)
	}
	c, err := store.Clone(ctx, client, *source, *approval)
	if err != nil {
		return err
	}
	return encode(result(c))
}
