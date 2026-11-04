package particlefixture

import (
	"fmt"

	"aego/format/aetext"
	"aego/modules/particles"
	"aego/vfs/osfs"
)

var Names = [...]string{"Dust", "Spark", "Smoke", "Rain", "Explosion"}

const cameraScene = `scene version=1 {
  node "77cf0000000000000000000000000001" name="Camera" enabled=true layers=1 {
    transform { position=[0, -0.55] rotation=0 scale=[1, 1] }
    component "engine.Camera2D" version=1 {
      zoom=47 viewport=[0, 0, 1, 1] mask=-1 order=0 clear=true
      clearColor=#1d2736ff scale=fitVirtual virtual=[256, 192] active=true
    }
  }
}`

func Write(root string) error {
	files := map[string]string{
		"project.aego": `project version=1 {
  name="Particle reference frames"
  module="game"
  mainScene=asset("77cf0000000000000000000000000100")
  native=false
  window { width=256 height=192 title="Particle reference frames" }
}
`,
		"scripts/demo/dust.aes": `package demo
script Landing {
    var frame int
    func Update(dt float) {
        frame++
        if frame == 48 { node.Get<ParticleSystem2D>().Burst(8) }
    }
}
`,
		"scripts/demo/dust.aes.meta": meta("77cf0000000000000000000000000200", "script"),
	}
	for i, name := range Names {
		data, err := particles.PresetSource(name)
		if err != nil {
			return err
		}
		p, err := aetext.Parse(data, name)
		if err != nil {
			return err
		}
		s, err := aetext.Parse([]byte(cameraScene), name)
		if err != nil {
			return err
		}
		n := p.Blocks[0]
		c := n.Block("component", particles.NameSystem)
		if c == nil {
			return fmt.Errorf("%s: missing particle component", name)
		}
		c.SetField("seed", aetext.Int(42))
		if name == "Dust" {
			script, err := aetext.Parse([]byte(`component "engine.Script" version=1 { script "game.demo.Landing" enabled=true {} }`), name)
			if err != nil {
				return err
			}
			n.Blocks = append(n.Blocks, script)
		}
		s.Blocks = append(s.Blocks, n)
		path := "scenes/" + name + ".aescene"
		files[path] = string(aetext.Write(s))
		files[path+".meta"] = meta(fmt.Sprintf("77cf000000000000000000000000%04x", 0x100+i), "scene")
	}
	out := osfs.Dir{Root: root}
	for p, data := range files {
		if err := out.WriteFile(p, []byte(data)); err != nil {
			return err
		}
	}
	return nil
}

func meta(id, imp string) string {
	return fmt.Sprintf("meta version=1 { id=%q importer=%q importerVersion=1 settings {} }\n", id, imp)
}
