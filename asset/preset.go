package asset

import (
	"errors"

	"aego/format/aetext"
)

func PresetComponent(n *aetext.Node, name string) (*aetext.Node, error) {
	var component *aetext.Node
	if n != nil && n.Head == "prefab" && len(n.Blocks) == 1 && n.Blocks[0].Head == "node" {
		for _, b := range n.Blocks[0].Blocks {
			switch b.Head {
			case "node":
				return nil, errors.New("component preset contains a child node")
			case "component":
				if component != nil {
					return nil, errors.New("expected a single preset component")
				}
				component = b
			}
		}
	}
	if component == nil || component.Label != name {
		return nil, errors.New("preset does not match the requested component")
	}
	return component, nil
}
