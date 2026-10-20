package mutate_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/asset"
	"aego/core/ids"
	"aego/scene/mutate"
	"aego/schema"
	"aego/script/bind"
)

type stats struct {
	Speed float32 `aego:"speed,min=0,max=10"`
	HP    int32   `aego:"hp"`
}

const sceneFile = "../../app/playground/examples/scenes/platformer.aescene"

func env(t *testing.T) mutate.Env {
	t.Helper()
	types, err := assembly.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builtin.Default().Load(types, bind.New(), assembly.Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := schema.RegisterData[stats](types.Schema(), schema.Name("test.Stats")); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	return mutate.Env{Types: types, Assets: func(id ids.AssetID) bool {
		_, ok := asset.LookupBuiltin(id)
		return ok
	}}
}

func load(t *testing.T, e mutate.Env) (*mutate.Document, []byte) {
	t.Helper()
	src, err := os.ReadFile(sceneFile)
	if err != nil {
		t.Fatal(err)
	}
	d, err := mutate.Load(src, "platformer.aescene", e)
	if err != nil {
		t.Fatal(err)
	}
	return d, d.Bytes()
}

func tx(ops ...mutate.Op) mutate.Tx {
	return mutate.Tx{Label: "test", Ops: ops}
}

var roundTrips = []struct {
	name string
	tx   mutate.Tx
	want string
}{
	{"set float", tx(mutate.Set("Player", "Body2D.gravityScale", "2")), "gravityScale=2"},
	{"set lane", tx(mutate.Set("Player", "SpriteRenderer.size.x", "2")), ""},
	{"set color", tx(mutate.Set("Floor", "SpriteRenderer.tint", "#ff0000ff")), "#ff0000ff"},
	{"set enum", tx(mutate.Set("Player", "physics.Body2D.type", "kinematic")), "type=kinematic"},
	{"set position", tx(mutate.Set("Player", "transform.position", "[3, -2]")), "position=[3, -2]"},
	{"set rotation lane free", tx(mutate.Set("Player", "transform.scale.y", "2")), ""},
	{"disable", tx(mutate.Set("Floor", "enabled", "false")), "enabled=false"},
	{"reset", tx(mutate.Reset("Floor", "SpriteRenderer.tint")), ""},
	{"rename", tx(mutate.Rename("Player", "Hero")), `name="Hero"`},
	{"add node", tx(mutate.AddNode("Root", "Platform").At(2)), `name="Platform"`},
	{"remove node", tx(mutate.RemoveNode("PlatformMid")), ""},
	{"move", tx(mutate.Move("PlatformLow", "Player").At(0)), ""},
	{"add comp", tx(mutate.AddComp("Floor", "engine.Tags")), "engine.Tags"},
	{"remove comp", tx(mutate.RemoveComp("Floor", "physics.BoxCollider2D")), ""},
	{"remove script", tx(mutate.RemoveComp("Player", "engine.Script")), ""},
	{"batch", tx(
		mutate.AddNode("Root", "Ledge"),
		mutate.Set("Ledge", "transform.position", "[0, 2]"),
		mutate.AddComp("Ledge", "SpriteRenderer"),
		mutate.Set("Ledge", "SpriteRenderer.size", "[4, 0.5]"),
		mutate.AddComp("Ledge", "Body2D"),
		mutate.AddComp("Ledge", "BoxCollider2D"),
		mutate.Set("Ledge", "BoxCollider2D.size", "[4, 0.5]"),
	), "size=[4, 0.5]"},
}

func TestApplyInverseRestoresBytes(t *testing.T) {
	e := env(t)
	for _, c := range roundTrips {
		t.Run(c.name, func(t *testing.T) {
			d, before := load(t, e)
			res, err := d.Apply(c.tx)
			if err != nil {
				t.Fatal(err)
			}
			after := d.Bytes()
			if bytes.Equal(after, before) {
				t.Fatal("the transaction changed nothing")
			}
			if c.want != "" && !strings.Contains(string(after), c.want) {
				t.Fatalf("the scene has no %s", c.want)
			}
			if len(res.Changes) != len(c.tx.Ops) {
				t.Fatalf("%d changes for %d ops", len(res.Changes), len(c.tx.Ops))
			}
			if _, err := d.Apply(res.Inverse); err != nil {
				t.Fatal(err)
			}
			if got := d.Bytes(); !bytes.Equal(got, before) {
				t.Fatalf("the inverse did not restore the scene:\n%s", got)
			}
		})
	}
}

func TestDryRunKeepsDocument(t *testing.T) {
	e := env(t)
	for _, c := range roundTrips {
		d, before := load(t, e)
		res, err := d.DryRun(c.tx)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !res.DryRun || len(res.Changes) == 0 {
			t.Fatalf("%s: result %+v", c.name, res)
		}
		if !bytes.Equal(d.Bytes(), before) {
			t.Fatalf("%s: dry run changed the document", c.name)
		}
	}
}

func TestFailedTransactionIsAtomic(t *testing.T) {
	e := env(t)
	bad := []mutate.Op{
		mutate.Set("Player", "Body2D.type", "flying"),
		mutate.Set("Player", "Body2D.noSuchField", "1"),
		mutate.Set("Nobody", "Body2D.mass", "1"),
		mutate.Set("Floor", "SpriteRenderer.sprite", `asset("ae9000000000000000000000000000ff")`),
		mutate.AddComp("Floor", "physics.BoxCollider2D"),
		mutate.RemoveNode("Root"),
		mutate.Set("Floor", "Body2D.gravityScale", "abc"),
		mutate.Set("Floor", "Body2D.gravityScale", "nan"),
		mutate.Set("Floor", "Body2D.gravityScale", `"2"`),
		mutate.Set("Floor", "SpriteRenderer.size", "[1]"),
		mutate.Set("Floor", "SpriteRenderer.size.x", "inf"),
		mutate.Set("Floor", "Collider2D.size", "[1, 1]"),
	}
	for _, op := range bad {
		d, before := load(t, e)
		_, err := d.Apply(tx(mutate.Rename("Player", "Hero"), mutate.AddNode("Root", "Extra"), op))
		if err == nil {
			t.Fatalf("%+v was accepted", op)
		}
		if !bytes.Equal(d.Bytes(), before) {
			t.Fatalf("%+v: a failed transaction changed the document", op)
		}
	}
}

func TestScriptRoundTrip(t *testing.T) {
	e := env(t)
	for _, c := range roundTrips {
		src, err := c.tx.Encode()
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		parsed, err := mutate.ParseTx(src, c.name+".aetext")
		if err != nil {
			t.Fatalf("%s: %v\n%s", c.name, err, src)
		}
		a, _ := load(t, e)
		b, _ := load(t, e)
		ra, err := a.Apply(c.tx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := b.Apply(parsed); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if c.name != "add node" && c.name != "batch" && !bytes.Equal(a.Bytes(), b.Bytes()) {
			t.Fatalf("%s: the parsed transaction differs", c.name)
		}
		inv, err := ra.Inverse.Encode()
		if err != nil {
			t.Fatal(err)
		}
		back, err := mutate.ParseTx(inv, "inverse.aetext")
		if err != nil {
			t.Fatalf("%s: inverse does not parse: %v\n%s", c.name, err, inv)
		}
		if _, err := a.Apply(back); err != nil {
			t.Fatalf("%s: parsed inverse: %v", c.name, err)
		}
	}
}

func TestInverseSurvivesReload(t *testing.T) {
	e := env(t)
	d, before := load(t, e)
	res, err := d.Apply(tx(mutate.Move("PlatformLow", "Player"), mutate.RemoveNode("PlatformMid"), mutate.AddNode("Root", "Extra")))
	if err != nil {
		t.Fatal(err)
	}
	inv, err := res.Inverse.Encode()
	if err != nil {
		t.Fatal(err)
	}
	again, err := mutate.Load(d.Bytes(), "platformer.aescene", e)
	if err != nil {
		t.Fatal(err)
	}
	back, err := mutate.ParseTx(inv, "inverse.aetext")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := again.Apply(back); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(again.Bytes(), before) {
		t.Fatal("an inverse applied after a reload did not restore the scene")
	}
}
