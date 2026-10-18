package settings

import (
	es "aego/engine/settings"
	"aego/schema"
	"aego/script/bind"
)

func installScript(m *bind.Module, mod *Module) {
	key := func(c *bind.Ctx, id string) (es.Key, bool) {
		if mod.st == nil {
			c.Fail("settings are not available here")
			return 0, false
		}
		k := mod.st.Key(id)
		if k == 0 {
			c.Failf("unknown setting %q", id)
		}
		return k, k != 0
	}
	set := func(c *bind.Ctx, id string, v schema.Value) {
		if k, ok := key(c, id); ok {
			if err := mod.tx().Set(k, v); err != nil {
				c.Fail(err.Error())
			}
		}
	}
	override := func(c *bind.Ctx, id string, v schema.Value) {
		if k, ok := key(c, id); ok {
			if err := mod.st.Override(k, v); err != nil {
				c.Fail(err.Error())
			}
		}
	}
	bind.Func1(m, "GetBool", func(c *bind.Ctx, id string) bool {
		k, ok := key(c, id)
		return ok && mod.st.Bool(k)
	}, bind.Doc("GetBool returns the effective value of an on/off setting."), bind.Params("id"),
		bind.Example("if settings.GetBool(\"accessibility.reducedMotion\") {"))
	bind.Func1(m, "GetInt", func(c *bind.Ctx, id string) int64 {
		if k, ok := key(c, id); ok {
			return mod.st.Int(k)
		}
		return 0
	}, bind.Doc("GetInt returns the effective value of an integer or choice setting."), bind.Params("id"),
		bind.Example("fps := settings.GetInt(\"display.frameLimit\")"))
	bind.Func1(m, "GetFloat", func(c *bind.Ctx, id string) float32 {
		if k, ok := key(c, id); ok {
			return float32(mod.st.Float(k))
		}
		return 0
	}, bind.Doc("GetFloat returns the effective value of a number setting."), bind.Params("id"),
		bind.Example("shake *= settings.GetFloat(\"accessibility.screenShake\")"))
	bind.Func1(m, "GetString", func(c *bind.Ctx, id string) string {
		if k, ok := key(c, id); ok {
			return mod.st.ChoiceName(k)
		}
		return ""
	}, bind.Doc("GetString returns a text setting, or the name of the current choice."), bind.Params("id"),
		bind.Example("mode := settings.GetString(\"display.window.mode\")"))
	bind.Proc2(m, "SetBool", func(c *bind.Ctx, id string, v bool) { set(c, id, schema.Bool(v)) },
		bind.Doc("SetBool stages a value; Apply makes it take effect."), bind.Params("id", "value"),
		bind.Example("settings.SetBool(\"accessibility.subtitles\", true)"))
	bind.Proc2(m, "SetInt", func(c *bind.Ctx, id string, v int64) { set(c, id, schema.Int64(v)) },
		bind.Doc("SetInt stages an integer or choice index."), bind.Params("id", "value"),
		bind.Example("settings.SetInt(\"display.frameLimit\", 60)"))
	bind.Proc2(m, "SetFloat", func(c *bind.Ctx, id string, v float32) { set(c, id, schema.Float64(float64(v))) },
		bind.Doc("SetFloat stages a number."), bind.Params("id", "value"),
		bind.Example("settings.SetFloat(\"audio.bus.music.volume\", 0.5)"))
	bind.Proc2(m, "SetString", func(c *bind.Ctx, id string, v string) { set(c, id, schema.String(v)) },
		bind.Doc("SetString stages a text value."), bind.Params("id", "value"),
		bind.Example("settings.SetString(\"display.window.resolution\", \"1920x1080\")"))
	bind.Proc1(m, "Reset", func(c *bind.Ctx, id string) {
		if k, ok := key(c, id); ok {
			mod.tx().Reset(k)
		}
	}, bind.Doc("Reset stages a return to the project default."), bind.Params("id"), bind.Example("settings.Reset(\"input.sensitivity\")"))
	bind.Func0(m, "Apply", func(c *bind.Ctx) bool {
		if mod.st == nil {
			return false
		}
		if err := mod.tx().Apply(); err != nil {
			c.Fail(err.Error())
			return false
		}
		return true
	}, bind.Doc("Apply applies every staged value at once; if one fails, none change."), bind.Example("if !settings.Apply() {"))
	bind.Proc0(m, "Revert", func(c *bind.Ctx) {
		if mod.st != nil {
			mod.tx().Revert()
		}
	}, bind.Doc("Revert drops the staged values."), bind.Example("settings.Revert()"))
	bind.Proc2(m, "OverrideFloat", func(c *bind.Ctx, id string, v float32) { override(c, id, schema.Float64(float64(v))) },
		bind.Doc("OverrideFloat sets a value for this session only; it is never saved."), bind.Params("id", "value"),
		bind.Example("settings.OverrideFloat(\"audio.bus.music.volume\", 0.2)"))
	bind.Proc2(m, "OverrideInt", func(c *bind.Ctx, id string, v int64) { override(c, id, schema.Int64(v)) },
		bind.Doc("OverrideInt sets an integer for this session only."), bind.Params("id", "value"), bind.Example("settings.OverrideInt(\"display.frameLimit\", 30)"))
	bind.Proc2(m, "OverrideBool", func(c *bind.Ctx, id string, v bool) { override(c, id, schema.Bool(v)) },
		bind.Doc("OverrideBool sets an on/off value for this session only."), bind.Params("id", "value"),
		bind.Example("settings.OverrideBool(\"audio.bus.sfx.mute\", true)"))
	bind.Proc2(m, "OverrideString", func(c *bind.Ctx, id string, v string) { override(c, id, schema.String(v)) },
		bind.Doc("OverrideString sets a text value for this session only."), bind.Params("id", "value"),
		bind.Example("settings.OverrideString(\"display.window.resolution\", \"1280x720\")"))
	bind.Proc1(m, "ClearOverride", func(c *bind.Ctx, id string) {
		if k, ok := key(c, id); ok {
			_ = mod.st.ClearOverride(k)
		}
	}, bind.Doc("ClearOverride removes a session override."), bind.Params("id"), bind.Example("settings.ClearOverride(\"audio.bus.music.volume\")"))
	bind.Proc0(m, "ClearOverrides", func(c *bind.Ctx) {
		if mod.st != nil {
			_ = mod.st.ClearOverrides()
		}
	}, bind.Doc("ClearOverrides removes every session override."), bind.Example("settings.ClearOverrides()"))
	bind.Func0(m, "NeedsRestart", func(c *bind.Ctx) bool {
		return mod.st != nil && mod.st.NeedsRestart()
	}, bind.Doc("NeedsRestart reports whether an applied setting takes effect only after a restart."), bind.Example("if settings.NeedsRestart() {"))
	bind.Proc0(m, "Save", func(c *bind.Ctx) {
		if mod.st != nil {
			if err := mod.st.Save(); err != nil {
				c.Fail(err.Error())
			}
		}
	}, bind.Doc("Save writes the player profile now; it is also saved on its own shortly after a change."), bind.Example("settings.Save()"))
	bind.Proc0(m, "OpenView", func(c *bind.Ctx) {
		if v := Of(c.World); v != nil {
			v.Open()
		}
	}, bind.Doc("OpenView shows the settings screen and pauses the game until it closes."), bind.Example("settings.OpenView()"))
	bind.Proc0(m, "CloseView", func(c *bind.Ctx) {
		if v := Of(c.World); v != nil {
			v.Close()
		}
	}, bind.Doc("CloseView hides the settings screen."), bind.Example("settings.CloseView()"))
	bind.Func0(m, "ViewOpen", func(c *bind.Ctx) bool {
		v := Of(c.World)
		return v != nil && v.Opened()
	}, bind.Doc("ViewOpen reports whether the settings screen is shown."), bind.Example("if settings.ViewOpen() {"))
}
