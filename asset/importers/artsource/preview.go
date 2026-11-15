package artsource

// WorkingSourcePreview opts into the existing transactional surface-generation
// builder. This is an importer capability, not a second asset pipeline.
func (*Importer) WorkingSourcePreview() bool { return true }
