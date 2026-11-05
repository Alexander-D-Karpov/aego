//go:build aego_golden

package golden

import (
	"fmt"
	"testing"

	"aego/core/mathx"
	"aego/editor/core/follow"
	"aego/editor/core/presence"
	"aego/editor/core/selection"
	"aego/editor/core/services"
	agentpanel "aego/editor/panels/agent"
	"aego/platform/input"
)

type toolbarGoldenHost struct{ *goldenHost }

func (*toolbarGoldenHost) Provider() string         { return "codex" }
func (*toolbarGoldenHost) SetProvider(string) error { return nil }
func (*toolbarGoldenHost) Model() string            { return "gpt-6.1-terra" }
func (*toolbarGoldenHost) SetModel(string) error    { return nil }
func (*toolbarGoldenHost) Effort() string           { return "" }
func (*toolbarGoldenHost) SetEffort(string) error   { return nil }

func TestEditorAgentToolbarV125(t *testing.T) {
	for _, width := range []int{420, 260} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			env := newEditorEnvSize(t, width, 360)
			f := follow.New(presence.New(), selection.New(), nopNav{})
			f.SetOn(true)
			f.UserActive(1)
			host := &toolbarGoldenHost{goldenHost: &goldenHost{}}
			p := agentpanel.New(agentpanel.Deps{Host: host, Follow: f, Root: t.TempDir(), Seconds: func() float64 { return 1 }})
			t.Cleanup(p.Close)
			img, err := env.draw(func(full mathx.Rect) { p.Draw(&services.Frame{UI: env.ui}, full) })
			checkGolden(t, fmt.Sprintf("editor_agent_toolbar_v125_%d", width), img, err)
		})
	}
}

func TestEditorAgentMentionsV125(t *testing.T) {
	env := newEditorEnvSize(t, 800, 600)
	host := &toolbarGoldenHost{goldenHost: &goldenHost{}}
	p := agentpanel.New(agentpanel.Deps{Host: host, Root: t.TempDir(), Assets: func() []string {
		return []string{"assets/anim/enemy.aectrl", "assets/anim/enemy_walk.aeanim", "assets/anim/player_walk.aeanim", "assets/sprites/player.png", "scripts/player/player.aes"}
	}})
	t.Cleanup(p.Close)
	p.Ask("")
	panel := mathx.Rect{W: 420, H: 520}
	ui := env.ui
	draw := func(_ mathx.Rect) { p.Draw(&services.Frame{UI: ui}, panel) }
	if _, err := env.draw(draw); err != nil {
		t.Fatal(err)
	}
	in := &input.State{}
	in.BeginFrame()
	in.ApplyText([]byte("@"))
	ui.Begin(in, mathx.V2(800, 600), 1)
	ui.Region(panel)
	p.Draw(&services.Frame{UI: ui}, panel)
	ui.PopCursor()
	ui.End()
	img, err := env.draw(draw)
	checkGolden(t, "editor_agent_mentions_v125", img, err)
}
