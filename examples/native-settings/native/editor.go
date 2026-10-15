//go:build aego_editor

package native

import "aego/sdk"

func registerEditor(r *sdk.Registry, m *Module) {
	sdk.EditorHook(r, func(e *sdk.EditorRegistry) error {
		e.GameOverlay(func(c *sdk.ShapeOverlay) {
			st := m.settings()
			if st == nil || c.Scene == nil || c.Mode3D {
				return
			}
			l := st.Direction.Len()
			if l == 0 {
				return
			}
			dir := st.Direction.Scale(c.Px * 40 / l)
			sdk.Query1[Wind](c.Scene).Each(func(n sdk.NodeHandle, _ *Wind) {
				p := c.Scene.WorldPosition(n)
				c.Debug.Arrow(p, p.Add(dir), sdk.Color{R: 0.4, G: 0.8, B: 1, A: 1})
			})
		})
		return nil
	})
}
