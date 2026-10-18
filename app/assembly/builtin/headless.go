package builtin

import "aego/app/assembly"

func Headless() assembly.Catalog {
	return assembly.Catalog{core(), inputEntry(), scriptEntry(), physicsEntry(), debugEntry(), displayEntry(), audioEntry(), accessibilityEntry(), settingsEntry()}
}
