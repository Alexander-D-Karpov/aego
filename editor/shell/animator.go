package shell

import (
	"path"
	"strings"

	"aego/asset"
	"aego/core/ids"
	"aego/editor/core/play"
	agentpanel "aego/editor/panels/agent"
	"aego/editor/panels/animator"
	"aego/editor/panels/sceneview"
	"aego/modules/animation"
	"aego/scene"
)

// animLive drives the Animator of the running game for the Animator panel.
type animLive struct{ a *App }

func (l animLive) find(node ids.NodeID) (*animation.Service, scene.NodeHandle, *scene.Scene) {
	pc := l.a.play
	sc := pc.Scene()
	if sc == nil {
		return nil, scene.NodeHandle{}, nil
	}
	return animation.Of(pc.World()), sc.Handle(node), sc
}

func (l animLive) State(node ids.NodeID) (string, float32, bool) {
	_, h, sc := l.find(node)
	if sc == nil || h.IsZero() {
		return "", 0, false
	}
	a, ok := scene.Get[animation.Animator](sc, h)
	if !ok {
		return "", 0, false
	}
	return a.State, a.Progress, true
}

func (l animLive) Param(node ids.NodeID, name string) (float64, bool) {
	s, h, _ := l.find(node)
	if s == nil || h.IsZero() {
		return 0, false
	}
	return s.Param(h, name)
}

func (l animLive) SetParam(node ids.NodeID, name string, v float64) {
	if s, h, _ := l.find(node); s != nil && !h.IsZero() {
		s.SetParam(h, name, v)
	}
}

func (a *App) newAnimator() *animator.Panel {
	a.animatorP = animator.New(animator.Deps{
		Docs: a.ctx, Selection: a.ctx, Notices: a.ctx, Files: a.ctx.AnimFiles,
		PathOf: func(id ids.AssetID) (string, bool) {
			if a.opt.Pipeline == nil {
				return "", false
			}
			return a.opt.Pipeline.PathOf(id)
		},
		Clips: a.animClips,
		Revision: func() uint64 {
			if a.opt.Pipeline == nil {
				return 0
			}
			return a.opt.Pipeline.Revision()
		},
		Live: func() animator.Live {
			if a.play.State() == play.Stopped || a.play.World() == nil {
				return nil
			}
			return animLive{a: a}
		},
	})
	return a.animatorP
}

// AnimatorPanel is the Animator panel, for tests and screenshots.
func (a *App) AnimatorPanel() *animator.Panel {
	return a.animatorP
}

func (a *App) animClips() []animator.Clip {
	if a.opt.Pipeline == nil {
		return nil
	}
	var out []animator.Clip
	for _, info := range a.opt.Pipeline.Assets() {
		if info.Kind == asset.ArtAnim {
			out = append(out, animator.Clip{ID: info.ID, Name: strings.TrimSuffix(path.Base(info.Path), path.Ext(info.Path))})
		}
	}
	return out
}

// SceneView is the scene view panel, for tests.
func (a *App) SceneView() *sceneview.Panel {
	return a.sceneView
}

// AgentCommand is the selected provider's binary, empty for its command from PATH.
func (a *App) AgentCommand() string {
	return a.AgentSettings("").Command
}

func (a *App) SetAgentCommand(cmd string) {
	s := a.AgentSettings("")
	s.Command = cmd
	a.SetAgentSettings(s)
}

// ProjectChanged follows a change of project.aego made outside Project Settings, such as the agent's project.set.
func (a *App) ProjectChanged() {
	if a.settings != nil {
		a.settings.Refresh()
	}
	a.liveSettings()
}

func (a *App) askHook() func(string) {
	if a.opt.Agent == nil {
		return nil
	}
	return func(prompt string) { a.agentP.Ask(prompt) }
}

// Agent is the Agent panel, nil without an agent host.
func (a *App) Agent() *agentpanel.Panel {
	return a.agentP
}

// assetPaths are the project's asset paths, for the Agent panel's @ mentions.
func (a *App) assetPaths() []string {
	if a.opt.Pipeline == nil {
		return nil
	}
	var out []string
	for _, info := range a.opt.Pipeline.Assets() {
		out = append(out, info.Path)
	}
	return out
}
