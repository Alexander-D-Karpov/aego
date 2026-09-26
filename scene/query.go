package scene

type Query1Of[A any] struct {
	s *Scene
	a *ComponentStore[A]
}

func Query1[A any](s *Scene) Query1Of[A] {
	return Query1Of[A]{s: s, a: storeOf[A](s)}
}

func (q Query1Of[A]) Each(fn func(NodeHandle, *A)) {
	q.a.Each(fn)
}

func (q Query1Of[A]) Count() int {
	n := 0
	q.a.Each(func(NodeHandle, *A) { n++ })
	return n
}

type Query2Of[A, B any] struct {
	s *Scene
	a *ComponentStore[A]
	b *ComponentStore[B]
}

func Query2[A, B any](s *Scene) Query2Of[A, B] {
	return Query2Of[A, B]{s: s, a: storeOf[A](s), b: storeOf[B](s)}
}

func (q Query2Of[A, B]) Each(fn func(NodeHandle, *A, *B)) {
	if q.a.Len() <= q.b.Len() {
		q.a.Each(func(h NodeHandle, a *A) {
			if b, ok := q.b.get(h); ok {
				fn(h, a, b)
			}
		})
		return
	}
	q.b.Each(func(h NodeHandle, b *B) {
		if a, ok := q.a.get(h); ok {
			fn(h, a, b)
		}
	})
}

func (q Query2Of[A, B]) Count() int {
	n := 0
	q.Each(func(NodeHandle, *A, *B) { n++ })
	return n
}

type Query3Of[A, B, C any] struct {
	s *Scene
	a *ComponentStore[A]
	b *ComponentStore[B]
	c *ComponentStore[C]
}

func Query3[A, B, C any](s *Scene) Query3Of[A, B, C] {
	return Query3Of[A, B, C]{s: s, a: storeOf[A](s), b: storeOf[B](s), c: storeOf[C](s)}
}

func (q Query3Of[A, B, C]) Each(fn func(NodeHandle, *A, *B, *C)) {
	q.a.Each(func(h NodeHandle, a *A) {
		b, ok := q.b.get(h)
		if !ok {
			return
		}
		c, ok := q.c.get(h)
		if !ok {
			return
		}
		fn(h, a, b, c)
	})
}

func (s *Scene) Query1[A any]() Query1Of[A] {
	return Query1[A](s)
}

func (s *Scene) Query2[A, B any]() Query2Of[A, B] {
	return Query2[A, B](s)
}

func (s *Scene) Query3[A, B, C any]() Query3Of[A, B, C] {
	return Query3[A, B, C](s)
}
