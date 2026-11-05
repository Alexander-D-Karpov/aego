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

// ClonePendingDefinition copies the latest queued material source on CPU so texture refresh preserves uncommitted edits; shader-only reloads return nil.
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
