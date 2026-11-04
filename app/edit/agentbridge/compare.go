package agentbridge

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	"aego/app/agent"
	"aego/app/agent/mcp"
)

type captureSetup struct {
	Scene, Camera, View, Input string
	Seed                       uint64
	Tick, Width, Height, Scale int
}
type visualReceipt struct {
	ID         string
	Generation uint64
	Setup      captureSetup
	PNG        []byte
}

func (b *Bridge) rememberShots(rep *agent.RunReport, gen uint64, scene string, req agent.RunRequest) {
	for i := range rep.Shots {
		s := &rep.Shots[i]
		if len(s.PNG) == 0 || len(s.PNG) > 5<<20 {
			continue
		}
		seed := req.Seed
		if seed == 0 {
			seed = 1
		}
		setup := captureSetup{scene, req.Camera, s.ViewSignature, req.Input, seed, s.Frame, s.Width, s.Height, req.ShotWidth}
		salt, _ := json.Marshal(setup)
		h := sha256.New()
		h.Write(salt)
		h.Write(s.PNG)
		s.CaptureID = fmt.Sprintf("%x", h.Sum(nil))
		if len(b.captures) >= 8 {
			copy(b.captures, b.captures[1:])
			b.captures = b.captures[:7]
		}
		b.captures = append(b.captures, visualReceipt{s.CaptureID, gen, setup, bytes.Clone(s.PNG)})
	}
}
func (b *Bridge) compareTool() mcp.Tool {
	str := map[string]any{"type": "string"}
	return mcp.Tool{Name: "scene_compare_frames", Description: "See two PNGs from scene_run shots side by side in call order. Both captureIDs must come from this conversation, with the same scene, camera, tick, seed, input and sizing.", ReadOnly: true, Schema: visualSchema([]string{"before", "after"}, map[string]any{"before": str, "after": str}), Output: func(a map[string]any) (*agent.Output, error) {
		var got [2]visualReceipt
		gen := b.panelGeneration.Load()
		if err := b.main(func() {
			for i, key := range []string{"before", "after"} {
				id, _ := a[key].(string)
				for _, r := range b.captures {
					if r.Generation == gen && r.ID == id {
						got[i] = r
						break
					}
				}
			}
		}); err != nil {
			return nil, err
		}
		x, y := got[0], got[1]
		if x.ID == "" || y.ID == "" {
			return nil, errors.New("capture missing or expired; repeat scene_run with shot")
		}
		if x.Setup != y.Setup {
			return nil, errors.New("capture settings differ; replay with matching scene, camera, tick, seed, input and scale")
		}
		return &agent.Output{OK: true, Value: map[string]any{"before": x.ID, "after": y.ID, "setup": x.Setup}, Images: []agent.Image{{Mime: "image/png", Data: x.PNG}, {Mime: "image/png", Data: y.PNG}}}, nil
	}}
}
