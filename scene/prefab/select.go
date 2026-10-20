package prefab

type Level uint8

const (
	LevelProperty Level = iota
	LevelComponent
	LevelNode
	LevelAll
	LevelOrphan
)

const (
	OwnerAdded  = "#node"
	OwnerRemove = "#remove"
	OwnerOrder  = "#order"
)

var levelNames = [...]string{"property", "component", "node", "all", "orphan"}

func (l Level) String() string {
	if int(l) < len(levelNames) {
		return levelNames[l]
	}
	return "all"
}

func ParseLevel(s string) (Level, bool) {
	if s == "" {
		return LevelAll, true
	}
	for i, n := range levelNames {
		if n == s {
			return Level(i), true
		}
	}
	return LevelAll, false
}

type Pick struct {
	Level Level
	Key   Key
	Prop  PropKey
}

func (p Pick) wants(orphan bool, k Key, owner, field string, rootKey Key) bool {
	if p.Level == LevelOrphan {
		if !orphan || k != p.Key {
			return false
		}
		return p.Prop.Owner == "" || (owner == p.Prop.Owner && (p.Prop.Field == "" || field == p.Prop.Field))
	}
	if orphan || (k == rootKey && owner == OwnerTransform) {
		return false
	}
	switch p.Level {
	case LevelProperty:
		return k == p.Key && owner == p.Prop.Owner && field == p.Prop.Field
	case LevelComponent:
		return k == p.Key && owner == p.Prop.Owner
	case LevelNode:
		return k == p.Key
	}
	return true
}

func (p Pick) Split(o Overrides, rootKey Key) (sel, rest Overrides) {
	put := func(take bool, fn func(*Overrides)) {
		if take {
			fn(&sel)
			return
		}
		fn(&rest)
	}
	for _, x := range o.Props {
		put(p.wants(x.Orphan, x.Node, x.Prop.Owner, x.Prop.Field, rootKey), func(t *Overrides) { t.Props = append(t.Props, x) })
	}
	for _, x := range o.AddComps {
		put(p.wants(x.Orphan, x.Node, x.Comp.Owner, "", rootKey), func(t *Overrides) { t.AddComps = append(t.AddComps, x) })
	}
	for _, x := range o.RemoveComps {
		put(p.wants(false, x.Node, x.Owner, "", rootKey), func(t *Overrides) { t.RemoveComps = append(t.RemoveComps, x) })
	}
	for _, x := range o.AddNodes {
		put(p.wants(x.Orphan, LocalKey(x.Node.Local), OwnerAdded, "", rootKey), func(t *Overrides) { t.AddNodes = append(t.AddNodes, x) })
	}
	for _, k := range o.Remove {
		put(p.wants(false, k, OwnerRemove, "", rootKey), func(t *Overrides) { t.Remove = append(t.Remove, k) })
	}
	for _, x := range o.Order {
		put(p.wants(false, x.Parent, OwnerOrder, "", rootKey), func(t *Overrides) { t.Order = append(t.Order, x) })
	}
	return sel, rest
}
