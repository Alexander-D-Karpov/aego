package player

import (
	"fmt"

	"aego/contracts/signals"
	"aego/core/ids"
	"aego/engine"
	"aego/script/bind"
	"aego/script/vm"
)

// CheckpointObservation retains the last actual Reached signal for one stable node.
type CheckpointObservation struct {
	Node  string `json:"node"`
	By    string `json:"by,omitempty"`
	Tick  uint64 `json:"tick"`
	Count uint64 `json:"count"`
}

func (p *Player) watchCheckpoint(detach func()) error {
	id, err := ids.ParseNodeID(p.cfg.ObserveCheckpoint)
	if err != nil {
		return err
	}
	s := p.Scene
	target := s.Handle(id)
	if target.IsZero() && p.checkpoint != nil {
		// A later scene need not contain the observed node.
		return nil
	}
	td, ok := s.Registry().ByName("gameplay.Checkpoint2D")
	if !ok || target.IsZero() {
		return fmt.Errorf("checkpoint observer: missing checkpoint %s in %s", id, p.sceneName())
	}
	if _, ok := s.GetByType(target, td.ID); !ok {
		return fmt.Errorf("checkpoint observer: node %s has no %s component", id, td.FullName)
	}
	world := p.Engine.MainWorld()
	hub, ok := engine.ServiceOf[signals.Hub](world, signals.ServiceID)
	if !ok {
		return fmt.Errorf("checkpoint observer: signal hub is unavailable")
	}
	for i, sig := range td.Signals {
		if sig.Name != "Reached" {
			continue
		}
		if p.checkpoint == nil {
			p.checkpoint = &CheckpointObservation{Node: id.String()}
		}
		src := signals.Source{Kind: signals.KindComponent, A: bind.NodeValue(target).A, B: uint64(td.ID)}
		conn := hub.Connect(src, i, func(args []vm.Value) {
			if len(args) == 1 {
				by := bind.NodeOf(args[0])
				if s.IsAlive(by) {
					p.checkpoint.By = s.ID(by).String()
					p.checkpoint.Tick = world.Time.Tick()
					p.checkpoint.Count++
				}
			}
		})
		if !hub.Connected(conn) {
			return fmt.Errorf("checkpoint observer: could not connect to %s.Reached", id)
		}
		p.detach = func() {
			hub.Disconnect(conn)
			detach()
		}
		return nil
	}
	return fmt.Errorf("checkpoint observer: %s.Reached is not registered", td.FullName)
}
