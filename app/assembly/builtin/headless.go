package builtin

import "aego/app/assembly"

func Headless() assembly.Catalog {
	return assembly.Catalog{core(), scriptEntry(), physicsEntry(), debugEntry()}
}
