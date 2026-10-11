package stdlib

import "aego/script/bind"

func installLog(r *bind.Registry) {
	m := r.Module(ModLog)
	bind.Proc1(m, "Debug", func(c *bind.Ctx, s string) {
		if c.Log != nil {
			c.Log.Debug(s)
		}
	}, bind.Doc("Debug writes a message to the Console. Debug messages are hidden unless the editor runs with -debug."),
		bind.Params("msg"), bind.Example("log.Debug(\"state \" + string(int(state)))"))
	bind.Proc1(m, "Info", func(c *bind.Ctx, s string) {
		if c.Log != nil {
			c.Log.Info(s)
		}
	}, bind.Doc("Info writes a message to the Console."), bind.Params("msg"), bind.Example("log.Info(\"ready \" + node.Name())"))
	bind.Proc1(m, "Warn", func(c *bind.Ctx, s string) {
		if c.Log != nil {
			c.Log.Warn(s)
		}
	}, bind.Doc("Warn writes a warning to the Console."), bind.Params("msg"), bind.Example("log.Warn(\"Target is not set\")"))
	bind.Proc1(m, "Error", func(c *bind.Ctx, s string) {
		if c.Log != nil {
			c.Log.Error(s)
		}
	}, bind.Doc("Error writes an error to the Console. It does not stop the script; use it for problems you can recover from."),
		bind.Params("msg"), bind.Example("log.Error(\"spawn point missing\")"))
}
