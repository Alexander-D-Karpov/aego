package shell

import (
	"slices"
	"strconv"
	"strings"

	"aego/editor/core/command"
	"aego/editor/core/context"
)

const CommandRepairMeta = "assets.repair_meta"

func (a *App) registerMeta() error {
	pl := a.opt.Pipeline
	if pl == nil {
		return nil
	}
	return a.ctx.Commands.Register(command.Desc{
		ID: CommandRepairMeta, Name: "Repair Meta Files…", Category: "Assets", Order: 300,
		Enabled: func() bool {
			l, _ := pl.MetaIssues()
			return len(l) > 0 && !pl.Shared()
		},
		Run: a.repairMeta,
	})
}

func (a *App) repairMeta() error {
	pl := a.opt.Pipeline
	list, _ := pl.MetaIssues()
	issues := slices.Clone(list)
	var b strings.Builder
	for i, is := range issues {
		if i == 8 {
			b.WriteString("and " + strconv.Itoa(len(issues)-i) + " more")
			break
		}
		b.WriteString(is.Action() + "\n")
	}
	a.ctx.Ask(context.Question{
		Title:   "Repair meta files",
		Message: b.String(),
		Buttons: []string{"Repair", "Cancel"},
		Then: func(choice int) {
			if choice != 0 {
				return
			}
			n, err := pl.Repair(issues)
			a.ctx.AssetRev++
			if err != nil {
				a.ctx.Notify(context.LevelError, err.Error())
				return
			}
			a.ctx.Notify(context.LevelInfo, "Repaired "+strconv.Itoa(n)+" meta issue(s)")
		},
	})
	return nil
}
