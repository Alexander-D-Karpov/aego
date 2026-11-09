---
id: data
icon: file.data
group: gameplay
title: Data assets
order: 7
---
# Data assets

A data asset holds numbers and references that tune the game without code: enemy stats, loot weights, level settings.

## Defining a type

Register a Go struct as a data type in a native module:

```go
package native

import (
	"aego/core/mathx"
	"aego/sdk"
)

type EnemyStats struct {
	Speed    float32     `aego:"speed,min=0,max=10"`
	Cooldown float32     `aego:"cooldown,min=0,max=5"`
	Damage   int64       `aego:"damage,min=0"`
	Tint     mathx.Color `aego:"tint"`
}

func registerData(r *sdk.Registry) {
	sdk.DataType[EnemyStats](r, sdk.Name("mygame.EnemyStats"))
	sdk.Live[EnemyStats](r, "speed")
}
```

Fields use the same tags and hints as component fields. Short type names (`EnemyStats`) must be unique across modules, because scripts name data types by them.

## Assets and bases

`.aedata` holds one record:

```
data version=1 type="mygame.EnemyStats" {
  speed=5
}
```

A record starts from the type defaults, copies its base when it has one and applies its own fields. Only fields that differ from the base are written. To inherit another record, add a `base` attribute containing that record's real AssetID; an ellipsis or file path is not a valid asset reference. A base must have the same type; a chain that comes back to itself or is deeper than 16 is an error.

In the editor, select a `.aedata` asset to edit it in the Inspector. Fields the asset sets are marked on the left; right-click one to revert it to the base. Save writes the file and reimports it.

## Tables

`.aetable` holds rows of one type, each with a unique id. Double-click a table to open the table editor. It sorts by a column (click its header), filters rows, edits cells inline and applies an edit to every selected row when the edited row is selected. Export CSV writes `<table>.csv` next to the table; Import CSV reads it back. Columns are the id and every bool, number, string, enum and asset field.

## Scripts

```
@export
var Stats AssetRef<EnemyStats>

@export
var Loot TableRef<LootEntry>

speed := Stats.Speed
copy := Stats.Copy()
if Loot.Has("goblin") {
    entry := Loot.Row("goblin")
}
n := Loot.Len()
```

Fields are read through the reference and cannot be assigned. `Copy()` returns a struct the script owns. A nil reference or a missing row stops the script with AES6004. Fields whose Go type has no script form, such as strings and lists, are not visible to scripts.

## Go

```go
package native

import "aego/sdk"

type EnemyStats struct {
	Speed float32 `aego:"speed,min=0,max=10"`
}

type Enemy struct {
	Stats sdk.DataRef[EnemyStats] `aego:"stats"`
}

func speedOf(ctx *sdk.Context, e *Enemy) float32 {
	stats, err := sdk.Data(ctx, e.Stats)
	if err != nil {
		return 0
	}
	return stats.Speed
}
```

`sdk.DataRef[T]` and `sdk.TableRef[T]` fields on components appear in the Inspector as asset pickers and can be passed to and from scripts.

## Live tuning

Fields are immutable while the game runs, except those marked with `sdk.Live`. During Play the Inspector lets you change live fields of a data asset; the change applies to the running game at once and to every asset based on it that does not set the field itself. When Play stops, the editor asks whether to keep the changes in the assets. Tables and non-live fields are read-only during Play.

## Validation

`aego check`, the Problems panel and Play report data assets that do not parse, name an unknown type, have a broken base, values outside their field's range or references to missing assets.
