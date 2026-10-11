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
	bind.Func0(m, "Float", func(c *bind.Ctx) float32 { return src(c).Float32() },
		bind.Doc("Float returns a random number in [0, 1)."), bind.Example("if rand.Float() < 0.25 {\n\tdropLoot()\n}"))
	bind.Func0(m, "Bool", func(c *bind.Ctx) bool { return src(c).Uint64()&1 == 1 },
		bind.Doc("Bool returns true or false with equal chance."), bind.Example("flip := rand.Bool()"))
	bind.Func2(m, "Int", func(c *bind.Ctx, lo, hi int64) int64 {
		if hi <= lo {
			return lo
		}
		return lo + int64(src(c).Uint64()%uint64(hi-lo))
	}, bind.Doc("Int returns a random int in [min, max). It returns min when max <= min."),
		bind.Params("min", "max"), bind.Example("roll := rand.Int(1, 7)"))
	bind.Func2(m, "Range", func(c *bind.Ctx, lo, hi float32) float32 { return src(c).Range(lo, hi) },
		bind.Doc("Range returns a random float in [min, max)."), bind.Params("min", "max"),
		bind.Example("delay := rand.Range(0.5, 1.5)"))
	bind.Func0(m, "Seed", func(c *bind.Ctx) int64 { return int64(src(c).Seed()) },
		bind.Doc("Seed returns the seed of the world's generator. Replays with the same seed produce the same numbers."),
		bind.Example("log.Info(\"seed \" + string(rand.Seed()))"))
}
