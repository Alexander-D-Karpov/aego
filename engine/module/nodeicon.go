package module

import "strconv"

func (b *ComponentBuilder[T]) NodeIcon(priority int) *ComponentBuilder[T] {
	d := b.edit()
	if d == nil {
		return b
	}
	if priority <= 0 {
		b.fail("component " + d.FullName + " has node icon priority " + strconv.Itoa(priority) + ", it must be positive")
		return b
	}
	d.NodeIcon = priority
	return b
}
