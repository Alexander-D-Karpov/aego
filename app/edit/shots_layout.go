package edit

import (
	"fmt"

	"aego/editor/core/context"
	"aego/editor/core/workspace"
	agentpanel "aego/editor/panels/agent"
	"aego/ui/dock"
)

// Expand the Inspector for screenshots that document its controls, then restore
// the split for other screenshots. Fail rather than silently rearrange a layout
// that does not have the expected dedicated Inspector/Agent split.
func expandScreenshotInspector(tree *dock.Tree) (func() error, error) {
	agent := tree.FindPanel(agentpanel.ID)
	if agent == 0 {
		if tree.Has(agentpanel.ID) {
			return nil, fmt.Errorf("screenshot Agent panel is not docked")
		}
		return func() error { return nil }, nil
	}
	n, _ := tree.Node(agent)
	inspector := tree.FindPanel(workspace.Inspector)
	parent, ok := tree.Node(n.Parent)
	if inspector == 0 || len(n.Tabs) != 1 || !ok || parent.Kind != dock.KindSplit || parent.Axis != dock.AxisY {
		return nil, fmt.Errorf("screenshot Inspector and Agent are not in a dedicated vertical split")
	}
	side := dock.SideBottom
	if parent.First == agent && parent.Second == inspector {
		side = dock.SideTop
	} else if parent.Second != agent || parent.First != inspector {
		return nil, fmt.Errorf("screenshot Agent is not next to the Inspector")
	}
	ratio := parent.Ratio
	if !tree.Undock(agentpanel.ID) {
		return nil, fmt.Errorf("could not expand the screenshot Inspector")
	}
	return func() error {
		anchor := tree.FindPanel(workspace.Inspector)
		if anchor == 0 || tree.Has(agentpanel.ID) {
			return fmt.Errorf("could not restore the screenshot Agent split")
		}
		tree.DockPanel(agentpanel.ID, anchor, side)
		restored, ok := tree.Node(tree.FindPanel(agentpanel.ID))
		if !ok {
			return fmt.Errorf("could not restore the screenshot Agent panel")
		}
		split, ok := tree.Node(restored.Parent)
		if !ok || split.Kind != dock.KindSplit || split.Axis != dock.AxisY {
			return fmt.Errorf("could not restore the screenshot Agent split")
		}
		split.Ratio = ratio
		return nil
	}, nil
}

// Match the status bar's eight-second notice visibility. Errors must not be
// concealed by waiting for them to expire or by a later informational notice.
func screenshotStatus(notices []context.Notice, now float64) (bool, error) {
	for _, n := range notices {
		if n.Level == context.LevelError {
			return false, fmt.Errorf("editor error during screenshot generation: %s", n.Text)
		}
	}
	if len(notices) == 0 {
		return false, nil
	}
	n := notices[len(notices)-1]
	return n.Level == context.LevelWarn && now-n.Time < 8, nil
}
