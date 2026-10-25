package edit

import (
	"aego/core/log"
	"aego/engine/i18n"
	"aego/engine/module"
)

// loadMessages adds the catalogs that modules register to the default bundle (D348).
func loadMessages(m *module.Manifest, lg *log.ChannelLogger) {
	if m == nil {
		return
	}
	for _, src := range m.Messages {
		if err := i18n.Default().LoadOnce(src.Owner, src.FS, ".", "i18n."+src.Owner); err != nil && lg != nil {
			lg.Warn("cannot read translations", log.F("owner", src.Owner), log.F("err", err.Error()))
		}
	}
}
