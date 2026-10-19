package state

import (
	"os"

	"aego/editor/core/document"
)

const FavoritesFile = "favorites.aetext"

func LoadFavorites(root string) ([]byte, bool) {
	data, err := os.ReadFile(userPath(root, FavoritesFile))
	return data, err == nil
}

func SaveFavorites(root string, data []byte) error {
	return document.WriteAtomic(userPath(root, FavoritesFile), data)
}
