package edit

import (
	"testing"

	"aego/editor/core/context"
	"aego/editor/core/workspace"
	agentpanel "aego/editor/panels/agent"
	"aego/ui/dock"
)

func TestScreenshotInspectorExpandsAndRestoresAgentSplit(t *testing.T) {
	tree := dock.Columns([]string{"left"}, []string{"browser"}, []string{"center"}, []string{"bottom"}, []string{workspace.Inspector})
	tree.DockPanel(agentpanel.ID, tree.FindPanel(workspace.Inspector), dock.SideBottom)
	agentNode, ok := tree.Node(tree.FindPanel(agentpanel.ID))
	if !ok {
		t.Fatal("agent is not docked")
	}
	split, ok := tree.Node(agentNode.Parent)
	if !ok {
		t.Fatal("agent split is missing")
	}
	split.Ratio = 0.55

	restore, err := expandScreenshotInspector(tree)
	if err != nil {
		t.Fatal(err)
	}
	if tree.Has(agentpanel.ID) || !tree.Has(workspace.Inspector) {
		t.Fatal("the agent still occupies the slicer inspector's space")
	}
	if err := restore(); err != nil {
		t.Fatal(err)
	}
	agentNode, ok = tree.Node(tree.FindPanel(agentpanel.ID))
	if !ok {
		t.Fatal("the agent was not restored")
	}
	split, ok = tree.Node(agentNode.Parent)
	if !ok || split.Axis != dock.AxisY || split.Ratio != 0.55 || split.Second != agentNode.ID || split.First != tree.FindPanel(workspace.Inspector) {
		t.Fatalf("the original inspector/agent split was not restored: %+v", split)
	}
}

func TestScreenshotInspectorRejectsUnexpectedAgentLayout(t *testing.T) {
	tree := dock.Standard(nil, []string{"center"}, nil, []string{workspace.Inspector, agentpanel.ID})
	if _, err := expandScreenshotInspector(tree); err == nil {
		t.Fatal("a shared tab group must not be removed as a dedicated agent split")
	}
	if !tree.Has(agentpanel.ID) || !tree.Has(workspace.Inspector) {
		t.Fatal("a rejected layout was changed")
	}
}

func TestScreenshotStatusWaitsForWarningAndRejectsError(t *testing.T) {
	for _, tc := range []struct {
		name    string
		notices []context.Notice
		now     float64
		wait    bool
		err     bool
	}{
		{name: "none"},
		{name: "information", notices: []context.Notice{{Level: context.LevelInfo, Time: 10}}, now: 10},
		{name: "recent warning", notices: []context.Notice{{Level: context.LevelWarn, Time: 10}}, now: 17.99, wait: true},
		{name: "expired warning", notices: []context.Notice{{Level: context.LevelWarn, Time: 10}}, now: 18},
		{name: "recent error", notices: []context.Notice{{Level: context.LevelError, Time: 10, Text: "failed import"}}, now: 11, err: true},
		{name: "expired error", notices: []context.Notice{{Level: context.LevelError, Time: 10}}, now: 18, err: true},
		{name: "error followed by info", notices: []context.Notice{{Level: context.LevelError, Text: "failed"}, {Level: context.LevelInfo}}, now: 30, err: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wait, err := screenshotStatus(tc.notices, tc.now)
			if wait != tc.wait || (err != nil) != tc.err {
				t.Fatalf("wait=%v err=%v, want wait=%v error=%v", wait, err, tc.wait, tc.err)
			}
		})
	}
}
