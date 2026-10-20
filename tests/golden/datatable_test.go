//go:build aego_golden

package golden

import (
	"testing"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/data"
	"aego/editor/core/dataedit"
	"aego/editor/core/services"
	"aego/editor/panels/datatable"
	"aego/engine/module"
	"aego/format/aetext"
	"aego/schema"
	"aego/script/bind"
	"aego/ui/dnd"
)

type goldenEnemy struct {
	Speed  float32 `aego:"speed,min=0,max=10"`
	Damage int64   `aego:"damage,min=0"`
	Flying bool    `aego:"flying"`
	Title  string  `aego:"title"`
}

type tableModule struct{}

func (tableModule) Info() module.Info {
	return module.Info{ID: "goldentable"}
}

func (tableModule) Register(r *module.Registry) error {
	return module.DataType[goldenEnemy](r, schema.Name("golden.Enemy")).Err()
}

func TestEditorDataTable(t *testing.T) {
	env := newEditorEnv(t)
	types, err := assembly.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builtin.Default().Load(types, bind.NewManifest(), assembly.Options{}, tableModule{}); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	row := func(id string, speed float32, damage int64, flying bool, title string) data.TableRow {
		return data.TableRow{ID: id, Fields: []aetext.Field{
			{Name: "speed", Value: aetext.Float32(speed)},
			{Name: "damage", Value: aetext.Int(damage)},
			{Name: "flying", Value: aetext.Bool(flying)},
			{Name: "title", Value: aetext.String(title)},
		}}
	}
	doc := &data.TableDoc{Type: "golden.Enemy", Rows: []data.TableRow{
		row("goblin", 2.5, 1, false, "Goblin"),
		row("bat", 4, 1, true, "Cave Bat"),
		row("troll", 1.2, 5, false, "Troll"),
		row("wisp", 6, 0, true, "Wisp"),
	}}
	tab, err := dataedit.NewTable(types.Schema(), ids.AssetID{Hi: 0xda7a, Lo: 1}, "assets/data/enemies.aetable", doc)
	if err != nil {
		t.Fatal(err)
	}
	p := datatable.New(datatable.Deps{})
	p.Show(tab)
	p.SetSort(1, true)
	p.Select("goblin", "troll")
	img, err := env.draw(func(full mathx.Rect) {
		p.Draw(&services.Frame{UI: env.ui, DnD: &dnd.State{}}, full)
	})
	checkGolden(t, "editor_data_table", img, err)
}
