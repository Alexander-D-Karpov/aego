package sceneio

import "testing"

func TestOutlineReadsNodesAndFields(t *testing.T) {
	src := []byte(`scene version=1 {
  node "a" name="Box" enabled=false layers=1 {
    transform {
      position=[0, -3]
    }
    component "physics.Body2D" version=1 {
      type=dynamic
      tint=#f59e0bff
    }
    node "b" name="Child" enabled=true layers=1 {
    }
  }
}
`)
	nodes, err := Outline(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].Name != "Box" || nodes[0].Enabled || len(nodes[0].Children) != 1 {
		t.Fatalf("nodes = %+v", nodes)
	}
	body := nodes[0].Blocks[1]
	if body.Type != "physics.Body2D" || body.Fields[0].Value != "dynamic" || body.Fields[1].Kind != "color" {
		t.Fatalf("component = %+v", body)
	}
	vals := OutlineValues(nodes)
	if vals[OutlineKey("a", nodes[0].Blocks[0], "position")] != "[0, -3]" {
		t.Fatalf("values = %v", vals)
	}
}
