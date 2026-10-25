package run

import (
	"bytes"
	"encoding/hex"
	"strconv"
	"strings"

	"aego/app/agent"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/rewind"
	"aego/engine/recorder"
	"aego/modules/audio"
	"aego/modules/camera"
	"aego/modules/gameplay"
	"aego/project"
	"aego/scene"
	"aego/sdk"
)

const maxRunLog = 200

func AgentRunner(mods ...sdk.Module) agent.Runner {
	return func(p *project.Project, r agent.RunRequest) (agent.RunReport, error) {
		var buf bytes.Buffer
		lg := log.New(log.NewTextSink(&buf))
		lg.SetLevel(log.Warn)
		frames, seed := r.Frames, r.Seed
		if frames <= 0 {
			frames = 120
		}
		if seed == 0 {
			seed = 1
		}
		a, err := Open(p, Config{Headless: true, FrameTime: defaultDetStep, Seed: seed, ScenePath: r.Scene, Logger: lg, NoProfile: true}, mods...)
		if err != nil {
			return agent.RunReport{Log: logLines(buf.String())}, err
		}
		rep := agent.RunReport{}
		driver := a.Driver()
		tracks := watch(a, r.Watch)
		sample := func() {
			for i := range tracks {
				if !tracks[i].h.IsZero() && a.Scene.IsAlive(tracks[i].h) {
					p := a.Scene.WorldPosition(tracks[i].h)
					tracks[i].Frames = append(tracks[i].Frames, rep.Frames)
					tracks[i].At = append(tracks[i].At, [2]float32{p.X, p.Y})
				}
			}
		}
		sample()
		for rep.Frames < frames {
			if !a.Engine.Step(driver) {
				rep.Error = "the game stopped at frame " + strconv.Itoa(rep.Frames+1)
				if err := a.Engine.Err(); err != nil {
					rep.Error += ": " + err.Error()
				}
				break
			}
			rep.Frames++
			if rep.Frames%max(1, frames/4) == 0 || rep.Frames == frames {
				sample()
			}
		}
		for _, t := range tracks {
			rep.Tracks = append(rep.Tracks, t.Track)
		}
		sum, _, _ := WorldHash(a.Engine.MainWorld(), rewind.NewHasher())
		rep.Hash = hex.EncodeToString(sum[:])
		rep.Counters = counters(a)
		a.Close()
		rep.Log = logLines(buf.String())
		return rep, nil
	}
}

func logLines(s string) []string {
	var out []string
	for _, l := range strings.Split(strings.TrimSpace(s), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	if len(out) > maxRunLog {
		out = append(out[:maxRunLog], "…")
	}
	return out
}

// counters sums what the modules counted during the run, for scene run and run_headless.
func counters(a *App) map[string]uint64 {
	out := map[string]uint64{}
	w := a.Engine.MainWorld()
	if s := gameplay.Of(w); s != nil {
		st := s.Stats()
		out["gameplay.steps"], out["gameplay.casts"], out["gameplay.hits"] = st.Steps, st.Casts, st.Hits
		out["gameplay.deaths"], out["gameplay.respawns"], out["gameplay.pickups"] = st.Deaths, st.Respawns, st.Pickups
	}
	if c := camera.Of(w); c != nil {
		c.Outputs(func(camera.OutputInfo) { out["camera.outputs"]++ })
	}
	if m := audio.ModuleOf(a.Engine); m != nil {
		c, st := m.Counts(), m.Stats()
		out["audio.finished"], out["audio.stolen"], out["audio.rejected"], out["audio.starved"] = c.Finished, c.Stolen, c.Rejected, c.Starved
		out["audio.underruns"] = st.Underruns
	}
	if r := w.Recorder; r != nil {
		counts := r.Counts()
		out["events.audio"], out["events.gameplay"] = counts[recorder.Audio], counts[recorder.Gameplay]
	}
	return out
}

type track struct {
	agent.Track
	h scene.NodeHandle
}

// watch finds the watched nodes in the running scene, by path from the root or #id.
func watch(a *App, paths []string) []track {
	out := make([]track, 0, len(paths))
	for _, p := range paths {
		t := track{Track: agent.Track{Node: p}}
		s := a.Scene
		if hex, ok := strings.CutPrefix(p, "#"); ok {
			if id, err := ids.ParseNodeID(hex); err == nil {
				t.h = s.Handle(id)
			}
		} else {
			t.h = s.Find(s.Root(), strings.TrimPrefix(strings.TrimPrefix(p, "/"), "Root/"))
		}
		if t.h.IsZero() || !s.IsAlive(t.h) {
			t.Error = "no node " + p + " in the running scene"
		}
		out = append(out, t)
	}
	return out
}
