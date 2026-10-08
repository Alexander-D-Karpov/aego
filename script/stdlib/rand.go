package stdlib

import (
	"aego/core/rng"
	"aego/script/bind"
)

func installRand(r *bind.Registry, seed uint64) {
	m := r.Module(ModRand)
	fallback := rng.New(seed)
	src := func(c *bind.Ctx) *rng.Source {
		if c.Random != nil {
			return c.Random
		}
		return fallback
	}
	bind.Func0(m, "Float", func(c *bind.Ctx) float32 { return src(c).Float32() })
	bind.Func0(m, "Bool", func(c *bind.Ctx) bool { return src(c).Uint64()&1 == 1 })
	bind.Func2(m, "Int", func(c *bind.Ctx, lo, hi int64) int64 {
		if hi <= lo {
			return lo
		}
		return lo + int64(src(c).Uint64()%uint64(hi-lo))
	})
	bind.Func2(m, "Range", func(c *bind.Ctx, lo, hi float32) float32 { return src(c).Range(lo, hi) })
	bind.Func0(m, "Seed", func(c *bind.Ctx) int64 { return int64(src(c).Seed()) })
}
