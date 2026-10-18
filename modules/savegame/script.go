package savegame

import (
	"aego/core/mathx"
	"aego/format/aetext"
	"aego/save"
	"aego/script/bind"
)

func serviceOf(c *bind.Ctx) *Service {
	s := Of(c.World)
	if s == nil {
		c.Fail("saving works only during Play")
	}
	return s
}

func writer(c *bind.Ctx) *save.Writer {
	s := serviceOf(c)
	if s == nil {
		return nil
	}
	if !s.capturing {
		c.Fail("save.Put works only inside OnSave")
		return nil
	}
	return s.writerFor(c.Node)
}

func reader(c *bind.Ctx) *save.Reader {
	if s := Of(c.World); s != nil {
		return s.readerFor(c.Node)
	}
	return nil
}

func report(c *bind.Ctx, err error) {
	if err != nil {
		c.Fail(err.Error())
	}
}

func installScript(m *bind.Module, mod *Module) {
	bind.Proc1(m, "Save", func(c *bind.Ctx, slot string) {
		if s := serviceOf(c); s != nil {
			report(c, s.Save(slot))
		}
	}, bind.Doc("Save writes the game to a slot at the end of the frame."), bind.Params("slot"), bind.Example("save.Save(\"slot1\")"))
	bind.Proc1(m, "Load", func(c *bind.Ctx, slot string) {
		if s := serviceOf(c); s != nil {
			report(c, s.Load(slot))
		}
	}, bind.Doc("Load reloads the saved scene and restores the slot at the end of the frame."), bind.Params("slot"), bind.Example("if save.Exists(\"slot1\") {\n  save.Load(\"slot1\")\n}"))
	bind.Proc0(m, "Checkpoint", func(c *bind.Ctx) {
		if s := serviceOf(c); s != nil {
			report(c, s.Checkpoint())
		}
	}, bind.Doc("Checkpoint saves the game to the slot named checkpoint."), bind.Example("save.Checkpoint()"))
	bind.Func1(m, "Exists", func(c *bind.Ctx, slot string) bool {
		s := Of(c.World)
		return s != nil && s.Exists(slot)
	}, bind.Doc("Exists reports whether a slot holds a save."), bind.Params("slot"))
	bind.Proc1(m, "Delete", func(c *bind.Ctx, slot string) {
		if s := serviceOf(c); s != nil {
			report(c, s.Delete(slot))
		}
	}, bind.Doc("Delete removes a save and its preview."), bind.Params("slot"))
	bind.Proc1(m, "Autosave", func(c *bind.Ctx, seconds float32) {
		if s := serviceOf(c); s != nil {
			s.SetAutosave(float64(seconds))
		}
	}, bind.Doc("Autosave saves to the slot named autosave every given number of real seconds; 0 turns it off."), bind.Params("seconds"))

	bind.Proc2(m, "PutInt", func(c *bind.Ctx, key string, v int64) {
		if w := writer(c); w != nil {
			w.Int(key, v)
		}
	}, bind.Doc("PutInt stores a value for this node; call it in OnSave."), bind.Params("key", "value"))
	bind.Proc2(m, "PutFloat", func(c *bind.Ctx, key string, v float32) {
		if w := writer(c); w != nil {
			w.Float(key, float64(v))
		}
	}, bind.Doc("PutFloat stores a value for this node; call it in OnSave."), bind.Params("key", "value"))
	bind.Proc2(m, "PutBool", func(c *bind.Ctx, key string, v bool) {
		if w := writer(c); w != nil {
			w.Bool(key, v)
		}
	}, bind.Doc("PutBool stores a value for this node; call it in OnSave."), bind.Params("key", "value"))
	bind.Proc2(m, "PutString", func(c *bind.Ctx, key, v string) {
		if w := writer(c); w != nil {
			w.String(key, v)
		}
	}, bind.Doc("PutString stores a value for this node; call it in OnSave."), bind.Params("key", "value"))
	bind.Proc2(m, "PutVec2", func(c *bind.Ctx, key string, v mathx.Vec2) {
		if w := writer(c); w != nil {
			w.Vec2(key, v)
		}
	}, bind.Doc("PutVec2 stores a value for this node; call it in OnSave."), bind.Params("key", "value"))

	bind.Func2(m, "GetInt", func(c *bind.Ctx, key string, def int64) int64 {
		return reader(c).Int(key, def)
	}, bind.Doc("GetInt reads a value this node stored, or def; it works in OnLoad."), bind.Params("key", "def"))
	bind.Func2(m, "GetFloat", func(c *bind.Ctx, key string, def float32) float32 {
		return float32(reader(c).Float(key, float64(def)))
	}, bind.Doc("GetFloat reads a value this node stored, or def; it works in OnLoad."), bind.Params("key", "def"))
	bind.Func2(m, "GetBool", func(c *bind.Ctx, key string, def bool) bool {
		return reader(c).Bool(key, def)
	}, bind.Doc("GetBool reads a value this node stored, or def; it works in OnLoad."), bind.Params("key", "def"))
	bind.Func2(m, "GetString", func(c *bind.Ctx, key, def string) string {
		return reader(c).String(key, def)
	}, bind.Doc("GetString reads a value this node stored, or def; it works in OnLoad."), bind.Params("key", "def"))
	bind.Func2(m, "GetVec2", func(c *bind.Ctx, key string, def mathx.Vec2) mathx.Vec2 {
		return reader(c).Vec2(key, def)
	}, bind.Doc("GetVec2 reads a value this node stored, or def; it works in OnLoad."), bind.Params("key", "def"))

	bind.Proc2(m, "SetGlobalInt", func(c *bind.Ctx, key string, v int64) {
		mod.SetGlobal(key, aetext.Int(v))
	}, bind.Doc("SetGlobalInt stores game-wide progress that every save carries."), bind.Params("key", "value"))
	bind.Proc2(m, "SetGlobalFloat", func(c *bind.Ctx, key string, v float32) {
		mod.SetGlobal(key, aetext.Float32(v))
	}, bind.Doc("SetGlobalFloat stores game-wide progress that every save carries."), bind.Params("key", "value"))
	bind.Proc2(m, "SetGlobalBool", func(c *bind.Ctx, key string, v bool) {
		mod.SetGlobal(key, aetext.Bool(v))
	}, bind.Doc("SetGlobalBool stores game-wide progress that every save carries."), bind.Params("key", "value"))
	bind.Proc2(m, "SetGlobalString", func(c *bind.Ctx, key, v string) {
		mod.SetGlobal(key, aetext.String(v))
	}, bind.Doc("SetGlobalString stores game-wide progress that every save carries."), bind.Params("key", "value"))
	bind.Func2(m, "GlobalInt", func(c *bind.Ctx, key string, def int64) int64 {
		if v, ok := mod.Global(key); ok && (v.Kind == aetext.VInt || v.Kind == aetext.VFloat) {
			return v.AsInt()
		}
		return def
	}, bind.Doc("GlobalInt reads game-wide progress, or def."), bind.Params("key", "def"), bind.Example("kills := save.GlobalInt(\"kills\", 0)"))
	bind.Func2(m, "GlobalFloat", func(c *bind.Ctx, key string, def float32) float32 {
		if v, ok := mod.Global(key); ok && (v.Kind == aetext.VInt || v.Kind == aetext.VFloat) {
			return float32(v.AsFloat())
		}
		return def
	}, bind.Doc("GlobalFloat reads game-wide progress, or def."), bind.Params("key", "def"))
	bind.Func2(m, "GlobalBool", func(c *bind.Ctx, key string, def bool) bool {
		if v, ok := mod.Global(key); ok && v.Kind == aetext.VBool {
			return v.Bool
		}
		return def
	}, bind.Doc("GlobalBool reads game-wide progress, or def."), bind.Params("key", "def"))
	bind.Func2(m, "GlobalString", func(c *bind.Ctx, key, def string) string {
		if v, ok := mod.Global(key); ok && v.Kind == aetext.VString {
			return v.Str
		}
		return def
	}, bind.Doc("GlobalString reads game-wide progress, or def."), bind.Params("key", "def"))
}
