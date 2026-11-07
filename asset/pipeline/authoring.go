package pipeline

// Structured source importers may opt into the existing working-generation
// transaction. Texture importers remain compatible without a new method.
// Only trusted, registered importer implementations can provide this capability.
type workingSourceImporter interface {
	WorkingSourcePreview() bool
}

func workingSourceCompatible(imp Importer) bool {
	if imp == nil {
		return false
	}
	if imp.ID() == "texture" {
		return true
	}
	capability, ok := imp.(workingSourceImporter)
	return ok && capability.WorkingSourcePreview()
}
