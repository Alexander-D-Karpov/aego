package assets

import (
	"os"
	"path"
	"regexp"
	"strings"

	"aego/core/ids"
	"aego/editor/context"
	"aego/editor/history/commands"
	"aego/scene/scriptcomp"
	"aego/schema"
)

var scriptDecl = regexp.MustCompile(`\bscript\s+([A-Za-z_][A-Za-z0-9_]*)\s*\{`)

func shortName(full string) string {
	return full[strings.LastIndexByte(full, '.')+1:]
}

func moduleOf(ctx *context.Editor) string {
	if ctx.Project != nil && ctx.Project.Module != "" {
		return ctx.Project.Module
	}
	return "game"
}

func ScriptPrefix(ctx *context.Editor, rel string) string {
	pkg := moduleOf(ctx)
	if dir := strings.TrimPrefix(strings.TrimPrefix(path.Dir(rel), "scripts"), "/"); dir != "" && dir != "." {
		pkg += "/" + dir
	}
	return strings.ReplaceAll(pkg, "/", ".")
}

func ScriptDir(ctx *context.Editor, typeName string) string {
	i := strings.LastIndexByte(typeName, '.')
	if i < 0 {
		return "scripts"
	}
	pkg := strings.ReplaceAll(typeName[:i], ".", "/")
	rel := strings.TrimPrefix(strings.TrimPrefix(pkg, moduleOf(ctx)), "/")
	if rel == "" {
		return "scripts"
	}
	return "scripts/" + rel
}

func ScriptTypesIn(ctx *context.Editor, id ids.AssetID) []string {
	if ctx.Pipeline == nil {
		return nil
	}
	rel, ok := ctx.Pipeline.PathOf(id)
	if !ok {
		return nil
	}
	src, err := os.ReadFile(ctx.Pipeline.Abs(rel))
	if err != nil {
		return nil
	}
	prefix := ScriptPrefix(ctx, rel)
	reg := ctx.Types.Schema()
	var out []string
	for _, m := range scriptDecl.FindAllSubmatch(src, -1) {
		full := prefix + "." + string(m[1])
		if td, ok := reg.ByName(full); ok && td.Kind == schema.TypeScript {
			out = append(out, td.FullName)
		}
	}
	return out
}

func AttachScript(ctx *context.Editor, nodes []ids.NodeID, typeName string) error {
	d := ctx.Doc()
	if d == nil || len(nodes) == 0 {
		return nil
	}
	s := d.Scene()
	tx := d.History.Begin("Add " + shortName(typeName))
	for _, n := range nodes {
		if sc, ok := scriptcomp.ScriptsOf(s, s.Handle(n)); ok && sc.Index(typeName) >= 0 {
			continue
		}
		if err := tx.Exec(&commands.AddScript{Node: n, TypeName: typeName}); err != nil {
			tx.Abort()
			return err
		}
	}
	tx.Commit()
	return nil
}

func AttachScriptAsset(ctx *context.Editor, nodes []ids.NodeID, id ids.AssetID) {
	types := ScriptTypesIn(ctx, id)
	nodes = append([]ids.NodeID(nil), nodes...)
	attach := func(name string) {
		if err := AttachScript(ctx, nodes, name); err != nil {
			ctx.Notify(context.LevelError, err.Error())
		}
	}
	switch len(types) {
	case 0:
		ctx.Notify(context.LevelWarn, "The file has no compiled script types, fix its errors first")
	case 1:
		attach(types[0])
	default:
		buttons := make([]string, 0, len(types)+1)
		for _, t := range types {
			buttons = append(buttons, shortName(t))
		}
		ctx.Ask(context.Question{
			Title:   "Attach script",
			Message: "This file declares several scripts. Which one should be attached?",
			Buttons: append(buttons, "Cancel"),
			Then: func(choice int) {
				if choice >= 0 && choice < len(types) {
					attach(types[choice])
				}
			},
		})
	}
}
