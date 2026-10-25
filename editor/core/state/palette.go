package state

import (
	"os"

	"aego/editor/core/document"
)

const PaletteFile = "palette.aetext"

func LoadPalette(root string) ([]byte, bool) {
	data, err := os.ReadFile(userPath(root, PaletteFile))
	return data, err == nil
}

func SavePalette(root string, data []byte) error {
	return document.WriteAtomic(userPath(root, PaletteFile), data)
}
