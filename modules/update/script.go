package update

import "aego/script/bind"

func installScript(m *bind.Module, mod *Module) {
	bind.Proc0(m, "Check", func(c *bind.Ctx) {
		mod.Check()
	}, bind.Doc("Check looks for a newer version on the game's update server in the background; State says what it found."),
		bind.Example("update.Check()"))
	bind.Func0(m, "State", func(c *bind.Ctx) string {
		p, _, _, _ := mod.State()
		return p.String()
	}, bind.Doc("State is idle, checking, upToDate, available, downloading, ready or failed."),
		bind.Example("if update.State() == \"ready\" {"))
	bind.Func0(m, "Version", func(c *bind.Ctx) string {
		_, v, _, _ := mod.State()
		return v
	}, bind.Doc("Version is the newer version the last check found, empty when there is none."),
		bind.Example("label.Text = \"Version \" + update.Version() + \" is ready\""))
	bind.Func0(m, "Notes", func(c *bind.Ctx) string {
		_, _, notes, _ := mod.State()
		return notes
	}, bind.Doc("Notes are the release notes of the newer version."),
		bind.Example("notes.Text = update.Notes()"))
	bind.Func0(m, "Error", func(c *bind.Ctx) string {
		_, _, _, err := mod.State()
		return err
	}, bind.Doc("Error says why the last check or download failed."),
		bind.Example("log.Warn(update.Error())"))
	bind.Proc0(m, "Download", func(c *bind.Ctx) {
		mod.Download()
	}, bind.Doc("Download fetches the newer version in the background; Auto mode does it on its own."),
		bind.Example("update.Download()"))
	bind.Proc0(m, "Apply", func(c *bind.Ctx) {
		if err := mod.Apply(); err != nil {
			c.Fail(err.Error())
		}
	}, bind.Doc("Apply installs the downloaded version and restarts the game on it; save the player's progress first."),
		bind.Example("if update.State() == \"ready\" { update.Apply() }"))
}
