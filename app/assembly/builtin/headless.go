package builtin

import "aego/app/assembly"

func Headless() assembly.Catalog {
	return assembly.Catalog{core(), dataEntry(), inputEntry(), scriptEntry(), physicsEntry(), debugEntry(), displayEntry(), audioEntry(), accessibilityEntry(), settingsEntry(), tweenEntry(), spawnEntry(), savegameEntry()}
}
