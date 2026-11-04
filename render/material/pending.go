package material

// Pending reports whether this material is waiting for a candidate to commit.
// Its Shader, values and Generation still describe the committed definition.
func (c *Cache) Pending(m *Material) bool {
	if c == nil || m == nil {
		return false
	}
	if c.isolated[m] != nil {
		return true
	}
	_, tracked := c.mats[m]
	return tracked && c.reloads[m.shader] != nil
}

// ClonePendingDefinition returns the latest queued material source, if any.
// A texture refresh must preserve a not-yet-committed material edit instead of
// rebuilding from the older live definition. The returned copy is CPU-only.
// A shader-only reload has no explicit material definition and returns nil.
func (c *Cache) ClonePendingDefinition(m *Material) (*Material, error) {
	if c == nil || m == nil {
		return nil, nil
	}
	if r := c.isolated[m]; r != nil {
		if def := r.definitions[m]; def != nil {
			return def.CloneDefinition()
		}
	}
	if r := c.reloads[m.shader]; r != nil {
		if def := r.definitions[m]; def != nil {
			return def.CloneDefinition()
		}
	}
	return nil, nil
}
