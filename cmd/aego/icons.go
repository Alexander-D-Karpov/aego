package main

import (
	"aego/engine/i18n"
	"bytes"
	"flag"
	"fmt"
	"os"

	"aego/app/assembly/builtin"
	"aego/app/bootstrap"
	"aego/app/projects"
	"aego/editor/core/iconuse"
	"aego/engine"
	"aego/engine/module"
)

func iconsUsage() string {
	return i18n.Tr("cli.icons.usage", `usage: aego icons [check|list] [dir] [flags]

  check            fail on icons that are used but missing, or provided but unused
  list [--json]    print the icon manifest as markdown, or as JSON
       [--out f]   write it to a file instead of stdout`)
}

func editorIcons() []module.IconUse {
	out := iconuse.Uses()
	// Module-provided editor icons belong in the application composition, not
	// in the module-independent editor's icon registry.
	out = append(out,
		module.IconUse{ID: "render2d.normals", Owner: iconuse.Owner, Size: 16, By: "Scene View: surface normals"},
		module.IconUse{ID: "render2d.point_light", Owner: iconuse.Owner, Size: 16, By: "Scene View: lighting preview"},
		module.IconUse{ID: "render2d.shadow_view", Owner: iconuse.Owner, Size: 16, By: "Scene View: shadow visibility"},
		module.IconUse{ID: "particles.system", Owner: iconuse.Owner, Size: 16, By: "Assets: New Effect command and Create palette"},
		module.IconUse{ID: "render2d.normals", Owner: iconuse.Owner, Size: 16, By: "Documentation navigation: normal mapping"},
		module.IconUse{ID: "render2d.point_light", Owner: iconuse.Owner, Size: 16, By: "Documentation navigation: platformer effects"},
		module.IconUse{ID: "animation.animator", Owner: iconuse.Owner, Size: 16, By: "Documentation navigation: animator"},
		module.IconUse{ID: "animation.player", Owner: iconuse.Owner, Size: 16, By: "Documentation navigation: animation guide and settings"},
	)
	for _, t := range projects.Templates {
		if t.Icon != "" {
			out = append(out, module.IconUse{ID: t.Icon, Owner: iconuse.Owner, By: "New Project: template " + t.ID, Size: 24})
		}
	}
	return out
}

func builtinAudit(extra []module.IconUse) (*module.IconReport, error) {
	return builtin.AuditIconsWith(extra)
}

func iconReport() (*module.IconReport, error) {
	return builtin.AuditIconsWith(editorIcons())
}

func iconsMarkdown() ([]byte, error) {
	rep, err := iconReport()
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	rep.WriteMarkdown(&b)
	return b.Bytes(), nil
}

func cmdIcons(args []string) int {
	sub := "check"
	if len(args) > 0 && (args[0] == "check" || args[0] == "list") {
		sub, args = args[0], args[1:]
	}
	fs := flag.NewFlagSet("icons "+sub, flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print JSON instead of markdown")
	out := fs.String("out", "", "write the manifest to this file")
	dir, rest := firstArg(args)
	if err := fs.Parse(rest); err != nil {
		fmt.Fprintln(os.Stderr, iconsUsage())
		return 2
	}
	if sub == "check" {
		if p, err := openProject(dir); err == nil && bootstrap.NeedsNative(p) {
			code, err := bootstrap.RunIcons(p, engine.Version, os.Stdout)
			if err != nil {
				return fail(err)
			}
			if code == 0 {
				fmt.Println("icons: ok")
			}
			return code
		} else if err != nil && dir != "" {
			return fail(err)
		}
	}
	rep, err := iconReport()
	if err != nil {
		return fail(err)
	}
	if sub == "check" {
		code := rep.Write(os.Stdout)
		if code == 0 {
			fmt.Println("icons: ok")
		}
		return code
	}
	var b bytes.Buffer
	if *asJSON {
		if err := rep.WriteJSON(&b); err != nil {
			return fail(err)
		}
	} else {
		rep.WriteMarkdown(&b)
	}
	if *out == "" {
		os.Stdout.Write(b.Bytes())
		return 0
	}
	if err := os.WriteFile(*out, b.Bytes(), 0o644); err != nil {
		return fail(err)
	}
	return 0
}
