package edit

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"aego/core/ids"
	"aego/editor/core/animedit"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/presence"
	"aego/editor/core/vectors"
	"aego/editor/core/workspace"
	animpanel "aego/editor/panels/animation"
	"aego/editor/panels/animator"
	"aego/editor/panels/gameview"
	"aego/editor/shell"
	"aego/ui/theme"
)

// EnvShots makes the editor write the documentation screenshots to this directory and exit.
const EnvShots = "AEGO_SHOTS"

type shot struct {
	name   string
	frames int
	inset  int
	bottom int
	setup  func(s *Session) error
}

func (s *Session) shots(dir string) error {
	defer s.Close()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if !s.Step() {
		return fmt.Errorf("the editor quit")
	}
	ctx := s.App.Context()
	scene := func() { ctx.Open(context.At("scenes/main.aescene", 1, 1)) }
	frameAll := func() error {
		if err := ctx.Commands.Run(command.EditSelectAll); err != nil {
			return err
		}
		if err := ctx.Commands.Run(command.SceneFrame); err != nil {
			return err
		}
		for i := 0; i < 30; i++ {
			s.Step()
		}
		return nil
	}
	list := []shot{
		{name: "editor", frames: 240, setup: func(s *Session) error {
			scene()
			s.App.Workspace().Show(workspace.Inspector)
			if err := frameAll(); err != nil {
				return err
			}
			return selectNamed(ctx, "Player")
		}},
		{name: "agent-follow", frames: 60, setup: func(s *Session) error {
			f := s.App.Follow()
			f.SetOn(true)
			now := s.Engine.Platform().Now()
			f.Step("scene_set", `{"file":"scenes/main.aescene","node":"Player","path":"Motion2D.maxSpeed","value":"9"}`, now)
			sc := ctx.Scene()
			player := sc.ID(sc.Find(sc.Root(), "Player"))
			s.App.Presence().Light(presence.Flash{Spot: presence.Spot{Doc: "scenes/main.aescene", Node: player, Type: "gameplay.Motion2D", Field: "maxSpeed"}, At: now})
			ctx.Select()
			return nil
		}},
		{name: "hitbox-gizmo", frames: 60, setup: func(s *Session) error {
			s.App.Follow().SetOn(false)
			s.App.Presence().Clear()
			s.App.AgentSelection().Clear()
			sc := ctx.Scene()
			var span []ids.NodeID
			for _, n := range []string{"EnemyLeft", "PatrolA1", "PatrolB1"} {
				span = append(span, sc.ID(sc.Find(sc.Root(), n)))
			}
			ctx.Select(span...)
			if err := ctx.Commands.Run(command.SceneFrame); err != nil {
				return err
			}
			for i := 0; i < 30; i++ {
				s.Step()
			}
			return selectNamed(ctx, "EnemyLeft")
		}},
		{name: "camera-follow", frames: 60, setup: func(s *Session) error {
			sc := ctx.Scene()
			ctx.Select(sc.ID(sc.Find(sc.Root(), "Camera")), sc.ID(sc.Find(sc.Root(), "Player")))
			if err := ctx.Commands.Run(command.SceneFrame); err != nil {
				return err
			}
			for i := 0; i < 30; i++ {
				s.Step()
			}
			return selectNamed(ctx, "Camera")
		}},
		{name: "camera-zone", frames: 60, setup: func(s *Session) error {
			sc := ctx.Scene()
			ctx.Select(sc.ID(sc.Find(sc.Root(), "ArenaZone")), sc.ID(sc.Find(sc.Root(), "PatrolA2")), sc.ID(sc.Find(sc.Root(), "PatrolB2")), sc.ID(sc.Find(sc.Root(), "GroundRight")))
			if err := ctx.Commands.Run(command.SceneFrame); err != nil {
				return err
			}
			for i := 0; i < 30; i++ {
				s.Step()
			}
			return selectNamed(ctx, "ArenaZone")
		}},
		{name: "clip-inspector", frames: 60, setup: func(s *Session) error {
			id, ok := s.Pipeline.Lookup("assets/audio/music.ogg")
			if !ok {
				return fmt.Errorf("no assets/audio/music.ogg")
			}
			s.App.Data().Inspect(id)
			return nil
		}},
		{name: "add-required", frames: 30, setup: func(s *Session) error {
			if err := selectNamed(ctx, "PatrolA1"); err != nil {
				return err
			}
			s.App.Inspector().OpenAdd("follow")
			return nil
		}},
		{name: "add-palette", frames: 30, setup: func(s *Session) error {
			s.App.AddPalette().Close()
			if err := selectNamed(ctx, "PatrolA1"); err != nil {
				return err
			}
			s.App.AddPalette().AddComponent(ctx.Selected(), nil)
			s.App.AddPalette().Palette().SetQuery("col")
			return nil
		}},
		{name: "tags", frames: 60, setup: func(s *Session) error {
			s.App.AddPalette().Close()
			ctx.UI.CloseAllPopups()
			return selectNamed(ctx, "Player")
		}},
		{name: "prefab", frames: 60, setup: func(s *Session) error {
			return selectNamed(ctx, "EnemyLeft")
		}},
		{name: "settings-input", frames: 60, setup: func(s *Session) error {
			ctx.Open(context.SettingsAt("input"))
			return nil
		}},
		{name: "settings-physics", frames: 60, setup: func(s *Session) error {
			ctx.Open(context.SettingsAt("physics"))
			return nil
		}},
		{name: "animator", frames: 60, setup: func(s *Session) error {
			scene()
			s.App.Workspace().Switch(workspace.Design)
			s.App.Workspace().Show(animator.ID)
			return selectNamed(ctx, "Player")
		}},
		{name: "attack-preview", frames: 30, setup: func(s *Session) error {
			scene()
			if err := selectNamed(ctx, "Player"); err != nil {
				return err
			}
			s.App.Workspace().Show(animpanel.ID)
			for range 5 {
				s.Step()
			}
			if !s.App.AnimationPanel().PickClip("assets/anim/player_hit.aeanim") {
				return fmt.Errorf("the Player has no player_hit clip")
			}
			for range 3 {
				s.Step()
			}
			if err := ctx.Commands.Run(command.SceneFrame); err != nil {
				return err
			}
			svc := ctx.Anim.For(ctx.Doc())
			svc.SetMode(animedit.Preview)
			svc.SetTime(0.17)
			return nil
		}},
		{name: "slicer", frames: 60, setup: func(s *Session) error {
			svc := ctx.Anim.For(ctx.Doc())
			svc.SetMode(animedit.Normal)
			for range 3 {
				s.Step()
			}
			if m := svc.Mode(); m != animedit.Normal {
				return fmt.Errorf("the clip is still in mode %d", m)
			}
			s.App.Workspace().Show(workspace.Inspector)
			id, ok := s.Pipeline.Lookup("assets/sprites/slash.png")
			if !ok {
				return fmt.Errorf("no assets/sprites/slash.png")
			}
			s.App.Data().Inspect(id)
			return nil
		}},
		{name: "script", frames: 90, setup: func(s *Session) error {
			ctx.Open(context.At("scripts/player/player.aes", 1, 1))
			return nil
		}},
		{name: "workspace-programming", frames: 60, setup: func(s *Session) error {
			s.App.Workspace().Switch(workspace.Programming)
			return nil
		}},
		{name: "play", frames: 150, setup: func(s *Session) error {
			s.App.Workspace().Switch(workspace.Design)
			scene()
			return ctx.Commands.Run(command.PlayStart)
		}},
		{name: "motion-play", frames: 30, setup: func(s *Session) error {
			s.App.Workspace().Show(workspace.Inspector)
			s.App.Inspector().ScrollTo("gameplay.Motion2D")
			return selectNamed(ctx, "Player")
		}},
		{name: "mixer", frames: 120, setup: func(s *Session) error {
			s.App.Workspace().Switch(workspace.Audio)
			s.App.Workspace().Show(workspace.Mixer)
			return nil
		}},
		{name: "physics-debug", frames: 30, setup: func(s *Session) error {
			s.App.Workspace().Switch(workspace.Design)
			ctx.Overlay.Set("physics.debug.contacts", true)
			return ctx.Commands.Run(gameview.CommandOverlays)
		}},
		{name: "demo", frames: 60, inset: 2, bottom: 28, setup: func(s *Session) error {
			for _, f := range []string{gameview.CommandOverlays, vectors.CommandToggle} {
				ctx.Overlay.Set(f, false)
			}
			return ctx.Commands.Run(shell.CommandFullscreenPlay)
		}},
		{name: "prefab-mode", frames: 120, setup: func(s *Session) error {
			if err := ctx.Commands.Run(command.PlayStop); err != nil {
				return err
			}
			id, ok := s.Pipeline.Lookup("prefabs/enemy.aeprefab")
			if !ok {
				return fmt.Errorf("no prefabs/enemy.aeprefab")
			}
			return s.App.Prefabs().Open(id)
		}},
	}
	if s.App.ThemeMode() == theme.ModeLight {
		for i := range list {
			list[i].name += "-light"
		}
	}
	for _, sh := range list {
		if err := sh.setup(s); err != nil {
			return fmt.Errorf("%s: %w", sh.name, err)
		}
		if err := s.capture(sh, filepath.Join(dir, sh.name+".png")); err != nil {
			return fmt.Errorf("%s: %w", sh.name, err)
		}
	}
	return nil
}

func selectNamed(ctx *context.Editor, name string) error {
	sc := ctx.Scene()
	if sc == nil {
		return fmt.Errorf("no scene is open")
	}
	h := sc.Find(sc.Root(), name)
	if !sc.IsAlive(h) {
		return fmt.Errorf("the scene has no node %q", name)
	}
	ctx.Select(sc.ID(h))
	return nil
}

func (s *Session) capture(sh shot, path string) error {
	for i := 0; i < sh.frames; i++ {
		if !s.Step() {
			return fmt.Errorf("the editor quit")
		}
	}
	var img *image.RGBA
	var err error
	s.Engine.CaptureFrame(func(got *image.RGBA, e error) { img, err = got, e })
	s.Step()
	if err != nil {
		return err
	}
	if img == nil {
		return fmt.Errorf("no frame was presented")
	}
	b := img.Bounds()
	crop := img.SubImage(image.Rect(b.Min.X+sh.inset, b.Min.Y+sh.inset, b.Max.X-sh.inset, b.Max.Y-sh.inset-sh.bottom))
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, crop)
}
